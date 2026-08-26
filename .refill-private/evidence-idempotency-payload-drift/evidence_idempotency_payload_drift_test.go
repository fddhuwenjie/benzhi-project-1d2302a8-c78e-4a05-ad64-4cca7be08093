package evidenceidempotencypayloaddrift_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/httpapi"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func TestEvidenceIdempotencyRejectsChangedPayload(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(workflow.New(repo, assessment.New(assessment.DefaultRules())))

	registered := request(t, handler, http.MethodPost, "/api/v1/transit-cases",
		`{"shipment_code":"IDEM-EDGE-1","container_code":"BOX-1","sample_category":"血清","temperature_min_c":2,"temperature_max_c":8}`,
		map[string]string{"X-Actor": "receiver", "X-Request-ID": "register-1", "Idempotency-Key": "register-key"})
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	var result workflow.CaseResult
	if err := json.Unmarshal(registered.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}

	path := "/api/v1/transit-cases/" + result.Case.ID + "/evidence"
	headers := map[string]string{
		"X-Actor": "receiver", "X-Request-ID": "evidence-1", "Idempotency-Key": "evidence-key", "If-Match": "1",
	}
	firstBody := evidenceBody(5)
	first := request(t, handler, http.MethodPost, path, firstBody, headers)
	if first.Code != http.StatusOK {
		t.Fatalf("first evidence status=%d body=%s", first.Code, first.Body.String())
	}

	headers["X-Request-ID"] = "evidence-retry-changed"
	changed := request(t, handler, http.MethodPost, path, evidenceBody(7), headers)
	if changed.Code != http.StatusConflict || !bytes.Contains(changed.Body.Bytes(), []byte(`"code":"idempotency_payload_conflict"`)) {
		t.Fatalf("changed payload reused cached success: status=%d body=%s", changed.Code, changed.Body.String())
	}
}

func evidenceBody(lastTemperature float64) string {
	return fmt.Sprintf(`{"transport_started_at":"2026-08-25T10:00:00Z","transport_ended_at":"2026-08-25T10:20:00Z","document_ref":"handoff://idem-edge-1","digest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","readings":[{"recorded_at":"2026-08-25T10:00:00Z","temperature_c":5,"sensor_serial":"S-1","source_batch":"B-1"},{"recorded_at":"2026-08-25T10:20:00Z","temperature_c":%g,"sensor_serial":"S-1","source_batch":"B-1"}]}`, lastTemperature)
}

func request(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
