package domain

type State string

const (
	StateDraft                State = "draft"
	StateEvidenceReady        State = "evidence_ready"
	StatePendingInvestigation State = "pending_investigation"
	StatePendingCorrection    State = "pending_correction"
	StatePendingVerification  State = "pending_verification"
	StateAssessmentPassed     State = "assessment_passed"
	StateClosed               State = "closed"
)

var allowedTransitions = map[State]map[State]bool{
	StateDraft:                {StateEvidenceReady: true},
	StateEvidenceReady:        {StatePendingInvestigation: true, StateAssessmentPassed: true},
	StatePendingInvestigation: {StatePendingCorrection: true, StateClosed: true},
	StatePendingCorrection:    {StatePendingVerification: true},
	StatePendingVerification:  {StatePendingCorrection: true, StateClosed: true},
	StateAssessmentPassed:     {StateClosed: true},
}

func CanTransition(from, to State) bool { return allowedTransitions[from][to] }
