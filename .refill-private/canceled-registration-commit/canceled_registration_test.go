package canceledregistrationcommit_test

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/httpapi"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func TestCanceledRegistrationDoesNotCommit(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	handler := httpapi.NewHandler(workflow.New(repo, assessment.New(assessment.DefaultRules())))
	body := []byte(`{"shipment_code":"CANCEL-001","container_code":"BOX-001","sample_category":"plasma","temperature_min_c":2,"temperature_max_c":8}`)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/transit-cases", bytes.NewReader(body))
	req.Header.Set("X-Actor", "receiver")
	req.Header.Set("X-Request-ID", "request-canceled")
	req.Header.Set("Idempotency-Key", "registration-canceled")
	ctx, cancel := context.WithCancel(req.Context())
	cancel()
	req = req.WithContext(ctx)

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)

	_, findErr := repo.FindByShipment("CANCEL-001")
	if !errors.Is(findErr, domain.ErrNotFound) {
		t.Fatalf("已取消的登记请求仍产生了持久化任务: status=%d find_err=%v", recorder.Code, findErr)
	}
}
