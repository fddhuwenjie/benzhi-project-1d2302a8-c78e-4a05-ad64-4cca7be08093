package audit_page_cache_stale_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/httpapi"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func TestAuditPageCacheTracksCommittedRevision(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(workflow.New(repo, assessment.New(assessment.DefaultRules())))

	registered := request(t, handler, http.MethodPost, "/api/v1/transit-cases",
		`{"shipment_code":"CACHE-AUDIT-1","container_code":"BOX-1","sample_category":"serum","temperature_min_c":2,"temperature_max_c":8}`,
		map[string]string{"X-Actor": "receiver", "X-Request-ID": "register-1", "Idempotency-Key": "register-1"})
	if registered.Code != http.StatusCreated {
		t.Fatalf("register status=%d body=%s", registered.Code, registered.Body.String())
	}
	var created workflow.CaseResult
	if err := json.Unmarshal(registered.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}

	auditPath := "/api/v1/transit-cases/" + created.Case.ID + "/audit?offset=0&limit=50"
	firstAudit := request(t, handler, http.MethodGet, auditPath, "", nil)
	assertAuditTotal(t, firstAudit, 1)

	start := time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC)
	evidenceBody := fmt.Sprintf(`{"transport_started_at":%q,"transport_ended_at":%q,"document_ref":"handoff://cache-audit-1","digest_sha256":"%s","readings":[{"recorded_at":%q,"temperature_c":4,"sensor_serial":"S-1","source_batch":"B-1"},{"recorded_at":%q,"temperature_c":5,"sensor_serial":"S-1","source_batch":"B-1"}]}`,
		start.Format(time.RFC3339), start.Add(30*time.Minute).Format(time.RFC3339),
		"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		start.Format(time.RFC3339), start.Add(30*time.Minute).Format(time.RFC3339))
	evidence := request(t, handler, http.MethodPost, "/api/v1/transit-cases/"+created.Case.ID+"/evidence", evidenceBody,
		map[string]string{"X-Actor": "receiver", "X-Request-ID": "evidence-1", "Idempotency-Key": "evidence-1", "If-Match": "1"})
	if evidence.Code != http.StatusOK || evidence.Header().Get("ETag") != `"2"` {
		t.Fatalf("evidence status=%d etag=%s body=%s", evidence.Code, evidence.Header().Get("ETag"), evidence.Body.String())
	}

	secondAudit := request(t, handler, http.MethodGet, auditPath, "", nil)
	assertAuditTotal(t, secondAudit, 2)
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

func assertAuditTotal(t *testing.T, response *httptest.ResponseRecorder, want int) {
	t.Helper()
	if response.Code != http.StatusOK {
		t.Fatalf("audit status=%d body=%s", response.Code, response.Body.String())
	}
	var page workflow.AuditPage
	if err := json.Unmarshal(response.Body.Bytes(), &page); err != nil {
		t.Fatal(err)
	}
	if page.Total != want || len(page.Items) != want {
		t.Fatalf("audit total=%d items=%d, want committed total=%d", page.Total, len(page.Items), want)
	}
}
