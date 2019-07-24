package main

import (
	"errors"
	"expvar"
	"log"
	"math/rand"
	"net/http"
	"os"
	"time"

	"github.com/sirupsen/logrus"
)

func work(log logrus.FieldLogger) error { // pretend work
	defer func(t time.Time) {
		log.WithField("func", "work").WithField("work_seconds", time.Since(t).Seconds()).Info("Work complete")
	}(time.Now())

	s := rand.Intn(99) + 1 // 1..100
	time.Sleep(time.Duration(s) * time.Millisecond)

	var err error
	if s <= 25 { // ~25% of the time the work errors
		err = errors.New("OMG Error!")
	}
	return err
}

func httpLogginghandler(log logrus.FieldLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := http.StatusOK // net/http returns 200 by default
		log = log.WithFields(logrus.Fields{
			"method": r.Method,
			"path":   r.URL.String(),
		})
		defer func(t time.Time) {
			log.WithField("func", "handler").WithField("status", status).WithField("duration", time.Since(t).Seconds()).Info()
		}(time.Now())

		if err := work(log); err != nil {
			status = http.StatusBadRequest
			http.Error(w, "Nope", status)
			log.Error("OMG Error!")
			return
		}

		w.Write([]byte(`:-)`))
	}
}

func main() {
	logrus.SetFormatter(&logrus.JSONFormatter{})
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

	// Expose port to expvar
	ex := expvar.NewString("Port")
	ex.Set(port)

	http.HandleFunc("/", httpLogginghandler(log))

	log.WithField("func", "main").Info("Listening at: http://localhost:" + port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatal("Errored with: " + err.Error())
	}
}
