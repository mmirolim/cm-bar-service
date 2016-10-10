package main

import (
	"cm-bar-service/cache"
	"cm-bar-service/queue"
	"crypto/md5"
	"encoding/json"
	"encoding/xml"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/golang/glog"
)

var (
	jobQueue           *queue.Queue
	mapPayloadToResult *cache.Cache

	hostPort      = flag.String("b", "localhost:3000", "The base URL of your service default http://localhost:3000")
	debug         = flag.Bool("debug", false, "Enable debug output")
	fooPort       = flag.Int("foo", 3001, "the port of the foo-service (default 3001)")
	workers       = flag.Int("w", 100, "number of workers default 100")
	jobQueueSize  = flag.Int("j", 1000, "number of jobs queued default 1000")
	fooHttpClient = &http.Client{
		Transport: &http.Transport{
			ResponseHeaderTimeout: 5 * time.Second,
		},
	}

	// set on compile time
	BuildVersion = "0.1"
)

func main() {
	flag.Parse()
	if *debug {
		flag.Set("alsologtostderr", "true")
		flag.Set("log_dir", "logs")
		flag.Set("v", "3")
	}

	defer glog.Flush()
	glog.Infof("Build Version %s", BuildVersion)

	// create queue and cache layer
	jobQueue = queue.NewQueue(*jobQueueSize, *workers, processJob)
	mapPayloadToResult = cache.NewCache()
	// register routes
	http.Handle("/job", middleware(PostJob))
	http.Handle("/job/", middleware(GetJob))
	srv := &http.Server{
		Addr:         *hostPort,
		ReadTimeout:  6 * time.Second,
		WriteTimeout: 6 * time.Second,
	}

	glog.Errorf("server is down %s", srv.ListenAndServe())
	jobQueue.Stop()
	glog.Infoln("exiting")
	os.Exit(1)
}

type JobRequest struct {
	N int `json:"n"`
}

type JobResponse struct {
	JobID string `json:"job_id"`
}

type JobResult struct {
	Status string `json:"status"`
	Result string `json:"result"`
	Error  error  `json:"error"`
}

type FooServiceResponse struct {
	XMLName xml.Name `xml:"fooServiceResponse"`
	Result  struct {
		Foo string `xml:"foo"`
	} `xml:"result"`
}

// middleware sets format type and logs exectime
func middleware(h http.HandlerFunc) http.Handler {
	fn := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		start := time.Now()
		defer func(s time.Time) {
			// TODO send to metrics
			glog.Infof("RA %s %f ms %s QUERY %s\n", r.RemoteAddr, time.Since(s).Seconds()*1000, r.Method+" "+r.URL.EscapedPath(), r.URL.RawQuery)
		}(start)
		h(w, r)

	}
	return http.HandlerFunc(fn)
}

// HandlerFunc to process job requests from clients
// Accepts only POST request with json content type
// Send job to queue and returns job id for polling
// if result for job payload already cached returns cache id
func PostJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Header["Content-Type"][0] == "application/json" {
		glog.V(3).Infof("postjob bad request method %s header %v", r.Method, r.Header)
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var jresp JobResponse
	var jreq JobRequest
	defer r.Body.Close()
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		glog.Errorf("postjob err %+v\n", err) // output for debug
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	glog.V(3).Infof("job posted %v", data)
	// compute hash from payload
	payloadHash := JobPayloadHash(data)
	// if already computed return id to get cahed result
	_, ok := mapPayloadToResult.Get(payloadHash)
	if ok {
		jresp.JobID = payloadHash
		glog.V(3).Infof("result exits for payload %v", payloadHash)
	} else {
		err = json.Unmarshal(data, &jreq)
		if err != nil {
			glog.Errorf("unmarshal %+v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		// blocks till there is slot in queue
		jresp.JobID = jobQueue.Create(data)
	}
	data, err = json.Marshal(&jresp)
	if err != nil {
		glog.Errorf("marshal err %+v\n", err)
		w.WriteHeader(http.StatusInternalServerError)
		return
	}

	glog.V(3).Infof("post response %+v", jresp)
	fmt.Fprintf(w, string(data))
}

// GetJob returns computed result for provided job id
// Accepts only GET method and returns result in json
// first check cached results then gets job status
// if status finished return result and cached it
func GetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	var jres JobResult
	// get jid from url
	url := strings.Split(r.URL.String(), "/")
	jid := url[len(url)-1]

	// check cached results for payloads
	res, ok := mapPayloadToResult.Get(jid)
	if ok {
		// return result
		glog.V(3).Infof("return cached result %+v\n", res)
		data, ok := res.([]byte)
		if !ok {
			glog.Errorf("cached result is not []byte %+v\n", res)
			w.WriteHeader(http.StatusInternalServerError)
			return

		}
		fmt.Fprintf(w, string(data))
		return
	}
	result, ok := jobQueue.Result(jid)
	if ok {
		jres.Status = result.Status
		jres.Error = result.Error
		jres.Result = string(result.Body)
		data, err := json.Marshal(&jres)
		if err != nil {
			glog.Errorf("marshal %+v\n", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}

		if jres.Status == queue.FinishedStatus {
			// cache computed job
			mapPayloadToResult.Put(JobPayloadHash(result.Payload), data)
		}

		if jres.Status == queue.ErrorStatus {
			glog.Errorf("failed job result %s result %+v", jid, jres)
		}

		glog.V(3).Infof("job result %s\n", data)
		fmt.Fprintf(w, string(data))
	} else {
		glog.Errorf("job not found %v", jid)
		w.WriteHeader(http.StatusNotFound)
	}
}

// retrieves result from foo-service
func processJob(j queue.Job) (result queue.Result) {
	result.Job.ID = j.ID
	result.Payload = j.Payload
	result.Status = queue.ErrorStatus

	var jobReq JobRequest
	err := json.Unmarshal(j.Payload, &jobReq)
	if err != nil {
		result.Error = err
		glog.Errorf("foo client unmarshal err %+v\n", err)
		return
	}

	url := fmt.Sprintf("http://localhost:%d/job/%d", *fooPort, jobReq.N)
	glog.V(3).Infof("req to foo-service %+v\n", url)

	resp, err := fooHttpClient.Post(url, "", nil)
	if err != nil {
		result.Error = err
		glog.Errorf("foo client err %+v\n", err)
		return
	}

	// TODO check content-type
	defer resp.Body.Close()
	data, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		result.Error = err
		glog.Errorf("read all err %+v %v\n", err, data)
		return
	}

	var fooResult FooServiceResponse
	err = xml.Unmarshal(data, &fooResult)
	if err != nil {
		result.Error = err
		glog.Errorf("xml unmarshal err %+v data %v", err, data)
		return
	}

	glog.V(3).Infof("fooresult %+v\n", string(data), fooResult)

	result.Status = queue.FinishedStatus
	result.Body = []byte(fooResult.Result.Foo)

	return
}

func JobPayloadHash(data []byte) string {
	return fmt.Sprintf("%x", md5.Sum(data))
}
