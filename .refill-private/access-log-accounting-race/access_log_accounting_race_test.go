package accesslogaccountingrace_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/httpapi"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

type barrierWriter struct {
	header  http.Header
	ready   chan<- struct{}
	release <-chan struct{}
}

func (w *barrierWriter) Header() http.Header { return w.header }

func (w *barrierWriter) WriteHeader(int) {
	w.ready <- struct{}{}
	<-w.release
}

func (w *barrierWriter) Write(body []byte) (int, error) { return len(body), nil }

func TestConcurrentAccessAccountingIsSynchronized(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(workflow.New(repo, assessment.New(assessment.DefaultRules())))
	ready := make(chan struct{}, 2)
	release := make(chan struct{})
	done := make(chan struct{}, 2)

	request := func() {
		writer := &barrierWriter{header: make(http.Header), ready: ready, release: release}
		handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/healthz", nil))
		done <- struct{}{}
	}
	go request()
	<-ready
	go request()
	<-ready
	close(release)
	<-done
	<-done
}
