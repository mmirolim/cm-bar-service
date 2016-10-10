package main

import (
	"bytes"
	"cm-bar-service/cache"
	"cm-bar-service/queue"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang/glog"
)

var (
	// test data
	expectedResult = "6ab1148f1969e07e4c729f5e2af199f2da9e3ebd5006dbe5d18081d0513286d9"
	fooSrvResp     = "<fooServiceResponse><result><foo>" + expectedResult + "</foo></result></fooServiceResponse>"
)

func TestPostJob(t *testing.T) {
	data := []byte(`{"n": 123456789}`)
	req, err := http.NewRequest("POST", "http://localhost:3000/job", bytes.NewBuffer(data))
	if err != nil {
		t.Errorf("postjob newrequest err %v", err)
	}
	w := httptest.NewRecorder()
	PostJob(w, req)
	var jres JobResponse
	data, err = ioutil.ReadAll(w.Body)
	if err != nil {
		t.Errorf("err %v", err)
	}
	err = json.Unmarshal(data, &jres)
	if w.Code != http.StatusOK || err != nil || jres.JobID == "" {
		t.Errorf("code %v job-id %s err %v ", w.Code, jres.JobID, err)
	}
}

func TestGetJob(t *testing.T) {
	data := []byte(`{"n": 123456789}`)
	req, err := http.NewRequest("POST", "http://localhost:3000/job", bytes.NewBuffer(data))
	if err != nil {
		t.Errorf("postjob newrequest err %v", err)
	}
	w := httptest.NewRecorder()
	PostJob(w, req)
	var jres JobResponse

	data, err = ioutil.ReadAll(w.Body)
	if err != nil {
		t.Errorf("err %v", err)
	}
	err = json.Unmarshal(data, &jres)

	req, err = http.NewRequest("GET", "http://localhost:3000/job/"+jres.JobID, nil)
	if err != nil {
		t.Errorf("err %v", err)
	}
	w = httptest.NewRecorder()
	// wait for response
	time.Sleep(time.Millisecond)
	GetJob(w, req)

	data, err = ioutil.ReadAll(w.Body)
	if err != nil {
		t.Errorf("err %v", err)
	}

	var result JobResult
	err = json.Unmarshal(data, &result)
	if err != nil {
		t.Errorf("postjob err %v", err)
	}
	if result.Result != expectedResult {
		t.Errorf("expected %v got %v", expectedResult, result)
	}

}

func TestFooServiceResponse(t *testing.T) {
	var fooResult FooServiceResponse
	err := xml.Unmarshal([]byte(fooSrvResp), &fooResult)
	if err != nil {
		t.Errorf("xml unmarshal err %v for %v", err, fooSrvResp)
	}

	if fooResult.Result.Foo != expectedResult {
		t.Errorf("expected %s got %s", expectedResult, fooResult.Result.Foo)
	}
}

func TestMain(m *testing.M) {
	defer glog.Flush()
	// create queue and cache layer
	jobQueue = queue.NewQueue(100, 20, processJob)
	mapPayloadToResult = cache.NewCache()
	go StartFooServiceMock()
	os.Exit(m.Run())
}

func StartFooServiceMock() {
	http.HandleFunc("/job/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, fooSrvResp)
	}))

	err := http.ListenAndServe("localhost:3001", nil)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
