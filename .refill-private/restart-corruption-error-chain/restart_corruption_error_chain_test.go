package restartcorruptionerrorchain_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func TestRestartPreservesCorruptAuditCause(t *testing.T) {
	dir := t.TempDir()
	repo, err := store.Open(dir)
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	tc, event, err := domain.NewTransitCase("case-restart", domain.NewCaseInput{
		ShipmentCode: "RESTART-1", ContainerCode: "BOX-1", SampleCategory: "serum",
		TemperatureMinC: 2, TemperatureMaxC: 8,
	}, domain.ChangeContext{Actor: "receiver", RequestID: "request-1", Now: now}, "event-1")
	if err != nil {
		t.Fatalf("new case: %v", err)
	}
	if err := repo.Create(tc, []domain.AuditEvent{event}, "register", "key-1", "fingerprint-1", tc); err != nil {
		t.Fatalf("create case: %v", err)
	}

	eventPath := filepath.Join(dir, "events.jsonl")
	raw, err := os.ReadFile(eventPath)
	if err != nil {
		t.Fatalf("read event log: %v", err)
	}
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Fatalf("expected complete event log before simulating truncation")
	}
	if err := os.WriteFile(eventPath, raw[:len(raw)-1], 0o640); err != nil {
		t.Fatalf("truncate final newline: %v", err)
	}

	_, err = store.Open(dir)
	if err == nil {
		t.Fatal("restart unexpectedly accepted a truncated audit log")
	}
	if !store.IsCorrupt(err) {
		t.Fatalf("restart lost the corrupt-log cause: %T: %v", err, err)
	}
}
