package partialcommit_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func TestFailedCommitDoesNotPublishAuditEvent(t *testing.T) {
	dir := t.TempDir()
	repo, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}

	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	change := domain.ChangeContext{Actor: "receiver", RequestID: "register", Now: now}
	tc, registered, err := domain.NewTransitCase("case-1", domain.NewCaseInput{
		ShipmentCode: "SHIP-1", ContainerCode: "BOX-1", SampleCategory: "serum",
		TemperatureMinC: 2, TemperatureMaxC: 8,
	}, change, "event-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Create(tc, []domain.AuditEvent{registered}, "register", "register-key", "fingerprint", tc); err != nil {
		t.Fatal(err)
	}

	snapshotPath := filepath.Join(dir, "snapshot.json")
	if err := os.Remove(snapshotPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(snapshotPath, 0o750); err != nil {
		t.Fatal(err)
	}

	next := tc
	evidenceReceived, err := next.Change("event-2", "evidence_received", domain.StateEvidenceReady, domain.ChangeContext{
		Actor: "receiver", RequestID: "evidence", Now: now.Add(time.Minute),
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.Commit(next, tc.Revision, store.Mutation{}, []domain.AuditEvent{evidenceReceived}, "evidence:case-1", "evidence-key", next); err == nil {
		t.Fatal("快照替换目标为目录时，Commit 应返回错误")
	}

	events, total, err := repo.Audit(tc.ID, 0, 50)
	if err != nil {
		t.Fatal(err)
	}
	if total != 1 || len(events) != 1 || events[0].EventID != registered.EventID {
		t.Fatalf("失败的 Commit 不应发布审计事件，得到 total=%d events=%+v", total, events)
	}
}
