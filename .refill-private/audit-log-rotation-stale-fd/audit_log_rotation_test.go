package auditlogrotation_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/httpapi"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func TestAuditAppendFollowsReplacedLog(t *testing.T) {
	dir := t.TempDir()
	repo, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(workflow.New(repo, assessment.New(assessment.DefaultRules())))

	registered := request(t, handler, http.MethodPost, "/api/v1/transit-cases",
		`{"shipment_code":"ROTATE-1","container_code":"BOX-1","sample_category":"plasma","temperature_min_c":2,"temperature_max_c":8}`,
		map[string]string{"X-Actor": "receiver", "X-Request-ID": "register-1", "Idempotency-Key": "register-key"})
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status = %d, body = %s", registered.Code, registered.Body.String())
	}
	var created workflow.CaseResult
	if err := json.Unmarshal(registered.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	eventPath := filepath.Join(dir, "events.jsonl")
	contents, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(eventPath, filepath.Join(dir, "events.jsonl.1")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(eventPath, contents, 0o640); err != nil {
		t.Fatal(err)
	}

	evidence := request(t, handler, http.MethodPost, "/api/v1/transit-cases/"+created.Case.ID+"/evidence",
		`{"transport_started_at":"2026-08-25T08:00:00Z","transport_ended_at":"2026-08-25T08:20:00Z","document_ref":"doc://handoff/rotate","digest_sha256":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","readings":[{"recorded_at":"2026-08-25T08:00:00Z","temperature_c":5,"sensor_serial":"S1","source_batch":"B1"},{"recorded_at":"2026-08-25T08:10:00Z","temperature_c":5,"sensor_serial":"S1","source_batch":"B1"},{"recorded_at":"2026-08-25T08:20:00Z","temperature_c":5,"sensor_serial":"S1","source_batch":"B1"}]}`,
		map[string]string{"X-Actor": "receiver", "X-Request-ID": "evidence-1", "Idempotency-Key": "evidence-key", "If-Match": `"1"`})
	if evidence.Code != http.StatusOK {
		t.Fatalf("evidence status = %d, body = %s", evidence.Code, evidence.Body.String())
	}
	var committed workflow.CaseResult
	if err := json.Unmarshal(evidence.Body.Bytes(), &committed); err != nil {
		t.Fatal(err)
	}
	if committed.Case.Revision != 2 {
		t.Fatalf("evidence revision = %d, want 2", committed.Case.Revision)
	}

	audit := request(t, handler, http.MethodGet, "/api/v1/transit-cases/"+created.Case.ID+"/audit", "", nil)
	if audit.Code != http.StatusOK {
		t.Fatalf("audit status = %d, body = %s", audit.Code, audit.Body.String())
	}
	var page workflow.AuditPage
	if err := json.Unmarshal(audit.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 {
		t.Fatalf("audit total = %d after successful revision 2 commit, want 2", page.Total)
	}
}

func request(t *testing.T, handler http.Handler, method, path, body string, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}
