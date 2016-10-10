This is bar-service which communicates with multiple
HTTP clients and one upstream service.
It  processes client requests asynchronously and assigns a job ID (string) for each client.
Service schedule requests in queue and process them with worker pools.
Computed jobs cached in the local cache to reduce pressure to upstream service (result are deterministic according to foo-service description).
Service has only one third-party dependency, it is https://godoc.org/github.com/golang/glog fast, convenient and leveled logger.
There are tests which test all handlers (TODO add parallel tests).

Issues:
During cm-test run with default configuration.
There are some requests to foo-service fails with EOF error.
There are client request timeouts with request canceled (Client.Timeout exceeded while awaiting headers).

TODO:
Instrument service with pprof, latency metrics, jobs in flight, upstream metrics.
Handling upstream service failure could be done with the limited rescheduling of jobs.

Scalling:
Service can be easily simplified and scaled if we remove the state from it and put behind load-balancer.
Cached could be done in Redis and keep queue in RabbitMQ (any caching or queuing system which can be clustered could be used).
If there is state related to a job then multiple instances can connect to each other keep list of available nodes and according to some job ID distribution (hashing payload) process jobs. But there are issues like orchestration, node failures, and hash collisions.
If there are multiple instances of upstream services (which are deterministic) then the list could be loaded with some configuration services/ENV/flags and kept health checked.

To test service
go test -v

Run Options:
-alsologtostderr
        log to standard error as well as files
  -b string
        The base URL of your service default http://localhost:3000 (default "localhost:3000")
  -debug
        Enable debug output
  -foo int
        the port of the foo-service (default 3001) (default 3001)
  -j int
        number of jobs queued default 1000 (default 1000)
  -log_backtrace_at value
        when logging hits line file:N, emit a stack trace
  -log_dir string
        If non-empty, write log files in this directory
  -logtostderr
        log to standard error instead of files
  -stderrthreshold value
        logs at or above this threshold go to stderr
  -v value
        log level for V logs
  -vmodule value
        comma-separated list of pattern=N settings for file-filtered logging
  -w int
        number of workers default 100 (default 100)
