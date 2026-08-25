package workflow

import (
	"testing"
	"time"

	"specimen-transit-guard/internal/assessment"
	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func TestCorrectionRejectedThenAccepted(t *testing.T) {
	repo, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	svc := New(repo, assessment.New(assessment.DefaultRules()))
	now := time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }
	registered, err := svc.Register(RegisterCommand{Metadata: Metadata{Actor: "receiver", RequestID: "r1", IdempotencyKey: "k1"},
		ShipmentCode: "SHIP-1", ContainerCode: "BOX-1", SampleCategory: "plasma", TemperatureMinC: 2, TemperatureMaxC: 8})
	if err != nil {
		t.Fatal(err)
	}
	start := now.Add(-time.Hour)
	evidenced, err := svc.ReceiveEvidence(EvidenceCommand{Metadata: Metadata{Actor: "receiver", RequestID: "r2", IdempotencyKey: "k2", ExpectedRevision: registered.Case.Revision},
		CaseID: registered.Case.ID, TransportStartedAt: start, TransportEndedAt: start.Add(20 * time.Minute), DocumentRef: "doc://handoff",
		DigestSHA256: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Readings: []domain.ReadingInput{
			{RecordedAt: start, TemperatureC: 5, SensorSerial: "S1", SourceBatch: "B1"},
			{RecordedAt: start.Add(10 * time.Minute), TemperatureC: 12, SensorSerial: "S1", SourceBatch: "B1"},
			{RecordedAt: start.Add(20 * time.Minute), TemperatureC: 5, SensorSerial: "S1", SourceBatch: "B1"}}})
	if err != nil {
		t.Fatal(err)
	}
	assessed, err := svc.Assess(AssessCommand{Metadata: Metadata{Actor: "quality", RequestID: "r3", IdempotencyKey: "k3", ExpectedRevision: evidenced.Case.Revision}, CaseID: registered.Case.ID})
	if err != nil {
		t.Fatal(err)
	}
	if assessed.Case.State != domain.StatePendingInvestigation {
		t.Fatalf("state = %s", assessed.Case.State)
	}
	investigated, err := svc.Investigate(InvestigateCommand{Metadata: Metadata{Actor: "quality", RequestID: "r4", IdempotencyKey: "k4", ExpectedRevision: assessed.Case.Revision},
		CaseID: registered.Case.ID, CauseCategory: "包装", RootCause: "冰袋失效", ImpactAnalysis: "样本稳定性可能下降", Disposition: "隔离待验证",
		NeedsCorrection: true, Assignee: "owner", DueAt: now.Add(24 * time.Hour), TriggerImpacts: map[string]string{"high_temperature_excursion": "稳定性存在风险"}})
	if err != nil {
		t.Fatal(err)
	}
	first, err := svc.SubmitCorrection(CorrectCommand{Metadata: Metadata{Actor: "owner", RequestID: "r5", IdempotencyKey: "k5", ExpectedRevision: investigated.Case.Revision},
		CaseID: registered.Case.ID, ActionText: "更换冰袋", CompletionNote: "已完成", EvidenceRefs: []string{"doc://photo/1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Verify(VerifyCommand{Metadata: Metadata{Actor: "owner", RequestID: "r-self", IdempotencyKey: "k-self", ExpectedRevision: first.Case.Revision},
		CaseID: registered.Case.ID, Accepted: true, Note: "自审"}); err != domain.ErrForbidden {
		t.Fatalf("整改责任人自审应被拒绝，实际错误为 %v", err)
	}
	rejected, err := svc.Verify(VerifyCommand{Metadata: Metadata{Actor: "verifier", RequestID: "r6", IdempotencyKey: "k6", ExpectedRevision: first.Case.Revision},
		CaseID: registered.Case.ID, Accepted: false, Note: "证据不完整", Issues: []domain.VerificationIssue{{ID: "issue-1", Category: "evidence", Description: "缺少校准记录"}}})
	if err != nil {
		t.Fatal(err)
	}
	if rejected.Case.State != domain.StatePendingCorrection {
		t.Fatalf("state = %s", rejected.Case.State)
	}
	svc.now = func() time.Time { return now.Add(25 * time.Hour) }
	if _, err := svc.SubmitCorrection(CorrectCommand{Metadata: Metadata{Actor: "owner", RequestID: "r-late", IdempotencyKey: "k-late", ExpectedRevision: rejected.Case.Revision},
		CaseID: registered.Case.ID, ActionText: "补充校准", CompletionNote: "校准完成", EvidenceRefs: []string{"doc://calibration/2"},
		IssueResolutions: []domain.IssueResolution{{IssueID: "issue-1", Resolution: "已补充校准记录"}}}); err == nil {
		t.Fatal("逾期整改缺少原因应被拒绝")
	}
	second, err := svc.SubmitCorrection(CorrectCommand{Metadata: Metadata{Actor: "owner", RequestID: "r7", IdempotencyKey: "k7", ExpectedRevision: rejected.Case.Revision},
		CaseID: registered.Case.ID, ActionText: "补充校准", CompletionNote: "校准完成", EvidenceRefs: []string{"doc://calibration/2"},
		IssueResolutions: []domain.IssueResolution{{IssueID: "issue-1", Resolution: "已补充校准记录"}}, OverdueReason: "等待校准服务商出具记录"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Deadline == nil || second.Deadline.Status != domain.DeadlineSubmittedLate || second.Deadline.OverdueMinutes != 60 {
		t.Fatalf("deadline = %+v", second.Deadline)
	}
	visible := true
	if _, err := svc.Verify(VerifyCommand{Metadata: Metadata{Actor: "quality", RequestID: "r-investigator", IdempotencyKey: "k-investigator", ExpectedRevision: second.Case.Revision},
		CaseID: registered.Case.ID, Accepted: true, Note: "调查人员自审", EvidenceVisible: &visible}); err != domain.ErrForbidden {
		t.Fatalf("调查提交人验证应被拒绝，实际错误为 %v", err)
	}
	closed, err := svc.Verify(VerifyCommand{Metadata: Metadata{Actor: "verifier", RequestID: "r8", IdempotencyKey: "k8", ExpectedRevision: second.Case.Revision},
		CaseID: registered.Case.ID, Accepted: true, Note: "证据完整", EvidenceVisible: &visible})
	if err != nil {
		t.Fatal(err)
	}
	if closed.Case.State != domain.StateClosed {
		t.Fatalf("state = %s", closed.Case.State)
	}
	summary, err := svc.Summary(registered.Case.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.ClosedEvent == nil || len(summary.CorrectiveActions) != 2 || len(summary.RawEvidenceRefs) != 4 || !summary.RevisionsContinuous {
		t.Fatalf("summary did not retain history: %+v", summary)
	}
	if len(summary.Completeness) != 9 || len(summary.EvidenceCatalog) != 4 || summary.RuleVersion != assessment.CurrentRuleVersion {
		t.Fatalf("summary completeness = %+v catalog=%+v", summary.Completeness, summary.EvidenceCatalog)
	}
	page, err := svc.Audit(registered.Case.ID, 0, 50)
	if err != nil || page.Total != 8 {
		t.Fatalf("audit total=%d err=%v", page.Total, err)
	}
}

func TestVerificationMustBeIndependent(t *testing.T) {
	// The full state-machine test above establishes an action owner. This assertion
	// is kept at the domain boundary through the public service behavior.
	repo, _ := store.Open(t.TempDir())
	svc := New(repo, assessment.New(assessment.DefaultRules()))
	if _, err := svc.Verify(VerifyCommand{CaseID: "missing"}); err == nil {
		t.Fatal("missing metadata must be rejected")
	}
}
