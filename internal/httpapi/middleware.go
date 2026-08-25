package httpapi

import (
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

type statusWriter struct {
	http.ResponseWriter
	status int
}

type accessStats struct {
	current int
	peak    int
}

var requestAccessStats accessStats

func (s *accessStats) enter() int {
	s.current++
	if s.current > s.peak {
		s.peak = s.current
	}
	return s.current
}

func (s *accessStats) leave() { s.current-- }

func (w *statusWriter) WriteHeader(code int) { w.status = code; w.ResponseWriter.WriteHeader(code) }

func recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				log.Printf("HTTP panic: %v\n%s", recovered, debug.Stack())
				writeError(w, errInternal{})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func accessMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		active := requestAccessStats.enter()
		defer requestAccessStats.leave()
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("method=%s path=%s status=%d active=%d duration=%s", r.Method, r.URL.Path, sw.status, active, time.Since(start))
	})
}

type errInternal struct{}

func (errInternal) Error() string { return "internal" }
