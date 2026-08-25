package workflow

import (
	"errors"
	"testing"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func TestEvidenceBatchesProgressAndReplay(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, assessment.New(assessment.DefaultRules()))
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	created, err := svc.Register(RegisterCommand{Metadata: Metadata{Actor: "receiver", RequestID: "r1", IdempotencyKey: "register"},
		ShipmentCode: "batch-1", ContainerCode: "box-1", SampleCategory: "serum", TemperatureMinC: 2, TemperatureMaxC: 8})
	if err != nil {
		t.Fatal(err)
	}
	start := now.Add(-time.Hour)
	first, err := svc.ReceiveEvidence(EvidenceCommand{Metadata: Metadata{Actor: "receiver", RequestID: "r2", IdempotencyKey: "batch-1", ExpectedRevision: created.Case.Revision},
		CaseID: created.Case.ID, TransportStartedAt: start, TransportEndedAt: start.Add(30 * time.Minute), Readings: []domain.ReadingInput{
			{RecordedAt: start, TemperatureC: 5, SensorSerial: "sensor-1", SourceBatch: "source-1"},
			{RecordedAt: start.Add(10 * time.Minute), TemperatureC: 6, SensorSerial: "sensor-1", SourceBatch: "source-1"},
		}})
	if err != nil {
		t.Fatal(err)
	}
	if first.Case.State != domain.StateDraft || first.EvidenceProgress == nil || first.EvidenceProgress.ReadingCount != 2 || len(first.EvidenceProgress.MissingItems) != 2 {
		t.Fatalf("first progress = %+v state=%s", first.EvidenceProgress, first.Case.State)
	}
	_, err = svc.ReceiveEvidence(EvidenceCommand{Metadata: Metadata{Actor: "receiver", RequestID: "duplicate", IdempotencyKey: "duplicate", ExpectedRevision: first.Case.Revision},
		CaseID: created.Case.ID, Readings: []domain.ReadingInput{{RecordedAt: start.Add(10 * time.Minute), TemperatureC: 7, SensorSerial: "sensor-2", SourceBatch: "source-2"}}})
	var fieldErr *domain.FieldError
	if !errors.As(err, &fieldErr) {
		t.Fatalf("duplicate reading error = %v", err)
	}
	secondCommand := EvidenceCommand{Metadata: Metadata{Actor: "receiver", RequestID: "r3", IdempotencyKey: "batch-2", ExpectedRevision: first.Case.Revision},
		CaseID: created.Case.ID, DocumentRef: "doc://handoff", DigestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
		Readings: []domain.ReadingInput{
			{RecordedAt: start.Add(20 * time.Minute), TemperatureC: 5, SensorSerial: "sensor-1", SourceBatch: "source-2"},
			{RecordedAt: start.Add(30 * time.Minute), TemperatureC: 5, SensorSerial: "sensor-1", SourceBatch: "source-2"},
		}}
	second, err := svc.ReceiveEvidence(secondCommand)
	if err != nil {
		t.Fatal(err)
	}
	if second.Case.State != domain.StateEvidenceReady || second.EvidenceProgress == nil || !second.EvidenceProgress.Ready || second.EvidenceProgress.ReadingCount != 4 {
		t.Fatalf("second progress = %+v state=%s", second.EvidenceProgress, second.Case.State)
	}
	replayed, err := svc.ReceiveEvidence(secondCommand)
	if err != nil || replayed.Case.Revision != second.Case.Revision {
		t.Fatalf("replay = %+v err=%v", replayed, err)
	}
	data, err := svc.Get(created.Case.ID)
	if err != nil || len(data.Readings) != 4 {
		t.Fatalf("readings=%d err=%v", len(data.Readings), err)
	}
	page, err := svc.Audit(created.Case.ID, 0, 50)
	if err != nil || page.Total != 3 {
		t.Fatalf("audit total=%d err=%v", page.Total, err)
	}
}
