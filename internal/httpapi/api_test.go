package httpapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func TestRegisterIdempotencyAndConflict(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := NewHandler(workflow.New(repo, assessment.New(assessment.DefaultRules())))
	body := []byte(`{"shipment_code":"HTTP-1","container_code":"BOX-1","sample_category":"血清","temperature_min_c":2,"temperature_max_c":8}`)
	first := perform(t, handler, http.MethodPost, "/api/v1/transit-cases", body, map[string]string{
		"X-Actor": "receiver", "X-Request-ID": "request-1", "Idempotency-Key": "same-key"})
	if first.Code != http.StatusCreated {
		t.Fatalf("status=%d body=%s", first.Code, first.Body.String())
	}
	var firstResult workflow.CaseResult
	if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatal(err)
	}
	second := perform(t, handler, http.MethodPost, "/api/v1/transit-cases", body, map[string]string{
		"X-Actor": "receiver", "X-Request-ID": "request-2", "Idempotency-Key": "same-key"})
	if second.Code != http.StatusCreated {
		t.Fatalf("replay status=%d", second.Code)
	}
	var replay workflow.CaseResult
	if err := json.Unmarshal(second.Body.Bytes(), &replay); err != nil {
		t.Fatal(err)
	}
	if replay.Case.ID != firstResult.Case.ID {
		t.Fatal("idempotent replay created a new case")
	}
	changed := []byte(`{"shipment_code":"HTTP-1","container_code":"BOX-1","sample_category":"血清","temperature_min_c":2,"temperature_max_c":9}`)
	mismatch := perform(t, handler, http.MethodPost, "/api/v1/transit-cases", changed, map[string]string{
		"X-Actor": "receiver", "X-Request-ID": "request-mismatch", "Idempotency-Key": "same-key"})
	if mismatch.Code != http.StatusConflict || !bytes.Contains(mismatch.Body.Bytes(), []byte(`"code":"idempotency_payload_conflict"`)) {
		t.Fatalf("mismatch status=%d body=%s", mismatch.Code, mismatch.Body.String())
	}
	duplicate := perform(t, handler, http.MethodPost, "/api/v1/transit-cases", body, map[string]string{
		"X-Actor": "receiver", "X-Request-ID": "request-3", "Idempotency-Key": "other-key"})
	if duplicate.Code != http.StatusConflict || !bytes.Contains(duplicate.Body.Bytes(), []byte(firstResult.Case.ID)) ||
		!bytes.Contains(duplicate.Body.Bytes(), []byte(`"current_revision":1`)) {
		t.Fatalf("duplicate status=%d body=%s", duplicate.Code, duplicate.Body.String())
	}
	get := perform(t, handler, http.MethodGet, "/api/v1/transit-cases/"+firstResult.Case.ID, nil, nil)
	if get.Code != http.StatusOK || get.Header().Get("ETag") != `"1"` {
		t.Fatalf("GET status=%d etag=%s", get.Code, get.Header().Get("ETag"))
	}
	audit := perform(t, handler, http.MethodGet, "/api/v1/transit-cases/"+firstResult.Case.ID+"/audit", nil, nil)
	if audit.Code != http.StatusOK || !bytes.Contains(audit.Body.Bytes(), []byte(`"total":1`)) {
		t.Fatalf("audit=%s", audit.Body.String())
	}
}

func perform(t *testing.T, handler http.Handler, method, path string, body []byte, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
