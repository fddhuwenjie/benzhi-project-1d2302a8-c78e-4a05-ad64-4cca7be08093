package httpapi

import (
	"net/http"
	"time"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/workflow"
)

func (a *API) AssessCase(w http.ResponseWriter, r *http.Request) {
	meta, err := metadata(r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.Assess(workflow.AssessCommand{Metadata: meta, CaseID: r.PathValue("id")})
	writeCommandResult(w, result, err)
}

type investigationRequest struct {
	CauseCategory      string            `json:"cause_category"`
	RootCause          string            `json:"root_cause"`
	ImpactAnalysis     string            `json:"impact_analysis"`
	Disposition        string            `json:"disposition"`
	NeedsCorrection    bool              `json:"needs_correction"`
	Assignee           string            `json:"assignee"`
	DueAt              time.Time         `json:"due_at"`
	ReviewReason       string            `json:"review_reason"`
	AcceptabilityBasis string            `json:"acceptability_basis"`
	TriggerImpacts     map[string]string `json:"trigger_impacts"`
}

func (a *API) InvestigateCase(w http.ResponseWriter, r *http.Request) {
	var body investigationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	meta, err := metadata(r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.Investigate(workflow.InvestigateCommand{Metadata: meta, CaseID: r.PathValue("id"), CauseCategory: body.CauseCategory,
		RootCause: body.RootCause, ImpactAnalysis: body.ImpactAnalysis, Disposition: body.Disposition, NeedsCorrection: body.NeedsCorrection,
		Assignee: body.Assignee, DueAt: body.DueAt, ReviewReason: body.ReviewReason, AcceptabilityBasis: body.AcceptabilityBasis,
		TriggerImpacts: body.TriggerImpacts})
	writeCommandResult(w, result, err)
}

type correctionRequest struct {
	ActionText       string                   `json:"action_text"`
	CompletionNote   string                   `json:"completion_note"`
	EvidenceRefs     []string                 `json:"evidence_refs"`
	OverdueReason    string                   `json:"overdue_reason"`
	IssueResolutions []domain.IssueResolution `json:"issue_resolutions"`
}

func (a *API) SubmitCorrectiveAction(w http.ResponseWriter, r *http.Request) {
	var body correctionRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	meta, err := metadata(r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.SubmitCorrection(workflow.CorrectCommand{Metadata: meta, CaseID: r.PathValue("id"), ActionText: body.ActionText,
		CompletionNote: body.CompletionNote, EvidenceRefs: body.EvidenceRefs, OverdueReason: body.OverdueReason, IssueResolutions: body.IssueResolutions})
	writeCommandResult(w, result, err)
}

type verificationRequest struct {
	Accepted        bool                       `json:"accepted"`
	Note            string                     `json:"note"`
	Issues          []domain.VerificationIssue `json:"issues"`
	EvidenceVisible *bool                      `json:"evidence_visible"`
}

func (a *API) VerifyCorrectiveAction(w http.ResponseWriter, r *http.Request) {
	var body verificationRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	meta, err := metadata(r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.Verify(workflow.VerifyCommand{Metadata: meta, CaseID: r.PathValue("id"), Accepted: body.Accepted, Note: body.Note,
		Issues: body.Issues, EvidenceVisible: body.EvidenceVisible})
	writeCommandResult(w, result, err)
}

type closeRequest struct {
	Reason string `json:"reason"`
}

func (a *API) CloseCase(w http.ResponseWriter, r *http.Request) {
	var body closeRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	meta, err := metadata(r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.ClosePassed(workflow.CloseCommand{Metadata: meta, CaseID: r.PathValue("id"), Reason: body.Reason})
	writeCommandResult(w, result, err)
}

func writeCommandResult(w http.ResponseWriter, result workflow.CaseResult, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", revisionETag(result.Case.Revision))
	writeJSON(w, http.StatusOK, result)
}
