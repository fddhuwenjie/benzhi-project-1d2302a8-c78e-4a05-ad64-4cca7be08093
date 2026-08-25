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
		sw := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(sw, r)
		log.Printf("method=%s path=%s status=%d duration=%s", r.Method, r.URL.Path, sw.status, time.Since(start))
	})
}

type errInternal struct{}

func (errInternal) Error() string { return "internal" }
