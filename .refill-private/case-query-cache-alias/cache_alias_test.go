package case_query_cache_alias_test

import (
	"testing"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
	"specimen-transit-guard/internal/workflow"
)

func TestCachedCaseQueryDoesNotLeakCallerMutation(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatalf("open repository: %v", err)
	}
	now := time.Date(2026, 8, 25, 9, 0, 0, 0, time.UTC)
	change := domain.ChangeContext{Actor: "receiver", RequestID: "register", Now: now}
	tc, registered, err := domain.NewTransitCase("case-cache", domain.NewCaseInput{
		ShipmentCode: "CACHE-001", ContainerCode: "BOX-001", SampleCategory: "blood",
		TemperatureMinC: 2, TemperatureMaxC: 8,
	}, change, "event-register")
	if err != nil {
		t.Fatalf("new case: %v", err)
	}
	if err := repo.Create(tc, []domain.AuditEvent{registered}, "", "", "", nil); err != nil {
		t.Fatalf("create case: %v", err)
	}

	next := tc
	evidenceEvent, err := next.Change("event-evidence", "evidence_received", domain.StateDraft, domain.ChangeContext{
		Actor: "receiver", RequestID: "evidence", Now: now.Add(time.Minute),
	}, nil)
	if err != nil {
		t.Fatalf("change case: %v", err)
	}
	originalTemperature := 4.5
	reading := domain.TemperatureReading{
		ID: "reading-1", TransitCaseID: tc.ID, RecordedAt: now,
		TemperatureC: originalTemperature, SensorSerial: "sensor-1", SourceBatch: "batch-1", ReceivedAt: now.Add(time.Minute),
	}
	if err := repo.Commit(next, tc.Revision, store.Mutation{Readings: []domain.TemperatureReading{reading}}, []domain.AuditEvent{evidenceEvent}, "", "", nil); err != nil {
		t.Fatalf("commit reading: %v", err)
	}

	service := workflow.New(repo, assessment.New(assessment.DefaultRules()))
	first, err := service.Get(tc.ID)
	if err != nil {
		t.Fatalf("first query: %v", err)
	}
	first.Readings[0].TemperatureC = 99

	second, err := service.Get(tc.ID)
	if err != nil {
		t.Fatalf("second query: %v", err)
	}
	if got := second.Readings[0].TemperatureC; got != originalTemperature {
		t.Fatalf("cached query leaked caller mutation: got %.1f, want %.1f", got, originalTemperature)
	}
}
