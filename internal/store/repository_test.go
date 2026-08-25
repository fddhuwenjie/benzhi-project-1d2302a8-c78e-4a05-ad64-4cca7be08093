package store

import (
	"testing"
	"time"

	"specimen-transit-guard/internal/domain"
)

func TestRepositoryPersistsAndChecksRevision(t *testing.T) {
	dir := t.TempDir()
	r, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	c, event, err := domain.NewTransitCase("c1", domain.NewCaseInput{ShipmentCode: "S1", ContainerCode: "B1", SampleCategory: "blood", TemperatureMinC: 2, TemperatureMaxC: 8}, domain.ChangeContext{Actor: "receiver", RequestID: "r1", Now: now}, "e1")
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Create(c, []domain.AuditEvent{event}, "register", "k1", "fingerprint", c); err != nil {
		t.Fatal(err)
	}
	next := c
	ev, err := next.Change("e2", "evidence_received", domain.StateEvidenceReady, domain.ChangeContext{Actor: "receiver", RequestID: "r2", Now: now.Add(time.Minute)}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Commit(next, 99, Mutation{}, []domain.AuditEvent{ev}, "", "", nil); err != domain.ErrConflict {
		t.Fatalf("want conflict, got %v", err)
	}
	r2, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := r2.FindByShipment("S1")
	if err != nil || got.Case.Revision != 1 {
		t.Fatalf("reload: %+v %v", got, err)
	}
}
