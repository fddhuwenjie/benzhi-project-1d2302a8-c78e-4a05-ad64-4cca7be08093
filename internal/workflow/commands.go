package workflow

import (
	"time"

	"specimen-transit-guard/internal/domain"
)

type Metadata struct {
	Actor, RequestID, IdempotencyKey string
	ExpectedRevision                 int64
}

type RegisterCommand struct {
	Metadata
	ShipmentCode, ContainerCode, SampleCategory string
	TemperatureMinC, TemperatureMaxC            float64
	RequestFingerprint                          string
}

type EvidenceCommand struct {
	Metadata
	CaseID             string
	TransportStartedAt time.Time
	TransportEndedAt   time.Time
	DocumentRef        string
	DigestSHA256       string
	Readings           []domain.ReadingInput
}

type AssessCommand struct {
	Metadata
	CaseID string
}

type InvestigateCommand struct {
	Metadata
	CaseID, CauseCategory, RootCause, ImpactAnalysis, Disposition string
	NeedsCorrection                                               bool
	Assignee, ReviewReason                                        string
	AcceptabilityBasis                                            string
	TriggerImpacts                                                map[string]string
	DueAt                                                         time.Time
}

type CorrectCommand struct {
	Metadata
	CaseID, ActionText, CompletionNote, OverdueReason string
	EvidenceRefs                                      []string
	IssueResolutions                                  []domain.IssueResolution
}

type VerifyCommand struct {
	Metadata
	CaseID          string
	Accepted        bool
	Note            string
	Issues          []domain.VerificationIssue
	EvidenceVisible *bool
}

type CloseCommand struct {
	Metadata
	CaseID, Reason string
}

type CaseResult struct {
	Case             domain.TransitCase          `json:"case"`
	Assessment       *domain.DeviationAssessment `json:"assessment,omitempty"`
	EvidenceProgress *domain.EvidenceProgress    `json:"evidence_progress,omitempty"`
	Deadline         *domain.DeadlineProjection  `json:"deadline,omitempty"`
}
