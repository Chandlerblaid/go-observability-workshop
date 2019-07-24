package main

import (
	"expvar"
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/sirupsen/logrus"
)

type myTimer struct {
	mu    sync.RWMutex
	count int
	sum   time.Duration
}

func (v *myTimer) Finish(t time.Time) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.sum += time.Since(t)
	v.count++
}

func (v *myTimer) String() string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	var avg int
	if v.count != 0 { // avoid divide by zero
		avg = int(v.sum) / v.count
	}
	return fmt.Sprintf(`{"Count": %d, "Sum": %d, "Avg": %d}`, v.count, v.sum, avg)
}

func work(log logrus.FieldLogger) error { // pretend work
	defer func(t time.Time) {
		log.WithField("work_seconds", time.Since(t).Seconds()).Info("Work complete")
	}(time.Now())

	s := rand.Intn(99) + 1 // 1..100
	time.Sleep(time.Duration(s) * time.Millisecond)

	var err error
	if s <= 25 { // ~25% of the time the work errors
		err = errors.New("OMG Error!")
	}
	return err
}

func timerMiddleware(t *myTimer, hf http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer t.Finish(time.Now())
		hf(w, r)
	}
}

func httpLoggingAndMetricsHandler(log logrus.FieldLogger, reqs, errs prometheus.Counter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK // net/http returns 200 by default
		log = log.WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.String(),
		})
		defer func(t time.Time) {
			reqs.Inc()
			log.WithField("status", status).WithField("duration", time.Since(t).Seconds()).Info()
		}(time.Now())

		if err := work(log); err != nil {
			status = http.StatusBadRequest
			errs.Inc()
			http.Error(w, "Nope", status)
			log.Error("OMG Error!")
			return
		}

		w.Write([]byte(`:-)`))
	}
}

func main() {
	logrus.SetFormatter(&logrus.TextFormatter{})
	f, err := os.OpenFile("testlogfile", os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
	if err != nil {
		log.Fatalf("error opening file: %v", err)
	}
	defer f.Close()

	// curried log
	logrus.SetOutput(f)
	log := logrus.WithField("app", "logs-02-server")

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Expose the port value
	ep := expvar.NewString("Port")
	ep.Set(port)

	// Export the numbers
	// errs := expvar.NewInt("Errors")
	reqs := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total http requests",
	})
	prometheus.MustRegister(reqs)
	errs := prometheus.NewCounter(prometheus.CounterOpts{
		Name: "http_error_total",
		Help: "Total http errors",
	})
	prometheus.MustRegister(errs)

	var t myTimer
	expvar.Publish("Requests", &t)

	http.Handle("/metrics", promhttp.Handler())
	http.HandleFunc("/", timerMiddleware(
		&t,
		httpLoggingAndMetricsHandler(log, reqs, errs),
	))

	log.Info("Listening at: http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Errored with: " + err.Error())
	}
}
