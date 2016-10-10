package queue

import (
	"cm-bar-service/cache"
	"crypto/md5"
	"fmt"
	"time"

	"github.com/golang/glog"
)

const (
	RunningStatus  = "running"
	FinishedStatus = "finished"
	ErrorStatus    = "error"
)

type Queue struct {
	maxJobs    int
	maxWorkers int
	job        chan Job
	quit       chan struct{}
	results    *cache.Cache
}

type Job struct {
	ID      string
	Payload []byte
}

type Result struct {
	Job
	Body   []byte
	Status string
	Error  error
}

func worker(fn func(Job) Result, jobs <-chan Job, results chan<- Result) {
	for j := range jobs {
		glog.V(3).Infof("processing job id %s payload %+v\n", j.ID, string(j.Payload))
		results <- fn(j)

	}
}

func NewQueue(size, workers int, fn func(Job) Result) *Queue {
	q := new(Queue)
	q.maxJobs = size
	q.maxWorkers = workers
	q.results = cache.NewCache()
	q.quit = make(chan struct{})
	q.job = make(chan Job)

	jobs := make(chan Job, q.maxJobs)
	results := make(chan Result, q.maxWorkers)
	// close jobs channel on quit
	go func() {
		<-q.quit
		close(jobs)
	}()
	// start routine to queue jobs
	go func() {
		for j := range q.job {
			select {
			case jobs <- j:
			case <-q.quit:
				glog.Infoln("exiting jobs listenning routine")
				return
			}
		}
	}()
	// start routine to store job results
	go func() {
		for {
			select {
			case r := <-results:
				q.results.Put(r.Job.ID, r)
				glog.V(3).Infof("update job result %+v\n", r)

			case <-q.quit:
				glog.Infoln("exiting job results listenning routine")
				return
			}
		}

	}()

	// start workers
	glog.V(3).Infof("start queue workers %v", q.maxWorkers)
	for w := 0; w < q.maxWorkers; w++ {
		go worker(fn, jobs, results)
	}
	glog.V(3).Infof("start queue with size %v", q.maxJobs)
	return q
}

func (q *Queue) Create(load []byte) string {
	now := time.Now().String()
	data := make([]byte, len(load)+len(now))
	n := copy(data[0:], load)
	copy(data[n:], now)
	job := Job{ID: fmt.Sprintf("%x", md5.Sum(data)), Payload: load}
	select {
	case q.job <- job:
		// store job as running
		q.results.Put(job.ID, Result{
			Job: Job{
				ID:      job.ID,
				Payload: job.Payload,
			},
			Status: RunningStatus,
			Body:   nil,
			Error:  nil,
		})
		glog.V(3).Infof("job scheduled as running id %s payload %s\n", job.ID, string(job.Payload))

	case <-q.quit:
		glog.Infoln("exiting queue create")
		break
	}

	return job.ID
}

func (q *Queue) Result(jid string) (Result, bool) {
	var result Result
	r, ok := q.results.Get(jid)
	glog.V(3).Infof("retrive job result jid %+v\n", jid) // output for debug

	if ok {
		result, ok = r.(Result)
		// delete finished|errored jobs
		if ok && result.Status != RunningStatus {
			glog.V(3).Infof("delete finished job %v", result)
			q.results.Del(result.Job.ID)
		}
		return result, ok
	} else {
		return result, false
	}
}

func (q *Queue) Stop() {
	glog.Infoln("stop all queue goroutines")
	q.quit <- struct{}{}
}
