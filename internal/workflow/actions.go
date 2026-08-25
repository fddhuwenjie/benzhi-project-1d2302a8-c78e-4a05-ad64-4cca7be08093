package workflow

import (
	"strings"
	"time"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func (s *Service) Investigate(cmd InvestigateCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "investigation:" + cmd.CaseID
	var prior CaseResult
	if ok, err := s.repo.Idempotent(scope, cmd.IdempotencyKey, "", &prior); ok || err != nil {
		return prior, err
	}
	data, err := s.repo.GetCase(cmd.CaseID)
	if err != nil {
		return CaseResult{}, err
	}
	if data.Case.Revision != cmd.ExpectedRevision {
		return CaseResult{}, domain.ErrConflict
	}
	if data.Case.State != domain.StatePendingInvestigation || data.Assessment == nil {
		return CaseResult{}, domain.ErrInvalidTransition
	}
	inv := domain.Investigation{
		TransitCaseID: cmd.CaseID, CauseCategory: strings.TrimSpace(cmd.CauseCategory), RootCause: strings.TrimSpace(cmd.RootCause),
		ImpactAnalysis: strings.TrimSpace(cmd.ImpactAnalysis), TriggerImpacts: copyStringMap(cmd.TriggerImpacts), Disposition: strings.TrimSpace(cmd.Disposition),
		NeedsCorrection: cmd.NeedsCorrection, ReviewReason: strings.TrimSpace(cmd.ReviewReason), AcceptabilityBasis: strings.TrimSpace(cmd.AcceptabilityBasis),
		ReviewedBy: ctx.Actor, ReviewedAt: ctx.Now,
	}
	if err := domain.ValidateInvestigationAgainstAssessment(inv, *data.Assessment, cmd.Assignee, cmd.DueAt); err != nil {
		return CaseResult{}, err
	}
	next := data.Case
	nextState := domain.StateClosed
	if cmd.NeedsCorrection {
		next.Assignee, next.DueAt = strings.TrimSpace(cmd.Assignee), timePointer(cmd.DueAt.UTC())
		nextState = domain.StatePendingCorrection
	}
	event, err := next.Change(newID("evt"), "investigation_completed", nextState, ctx, map[string]any{
		"assessment_id": data.Assessment.ID, "severity": data.Assessment.Severity, "investigation": inv,
		"assignee": next.Assignee, "due_at": next.DueAt,
	})
	if err != nil {
		return CaseResult{}, err
	}
	result := CaseResult{Case: next, Assessment: data.Assessment}
	err = s.repo.Commit(next, cmd.ExpectedRevision, store.Mutation{Investigation: &inv}, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, result)
	return result, err
}

func (s *Service) SubmitCorrection(cmd CorrectCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "correction:" + cmd.CaseID
	var prior CaseResult
	if ok, err := s.repo.Idempotent(scope, cmd.IdempotencyKey, "", &prior); ok || err != nil {
		return prior, err
	}
	data, err := s.repo.GetCase(cmd.CaseID)
	if err != nil {
		return CaseResult{}, err
	}
	if data.Case.Revision != cmd.ExpectedRevision {
		return CaseResult{}, domain.ErrConflict
	}
	if data.Case.State != domain.StatePendingCorrection || data.Investigation == nil || data.Case.DueAt == nil {
		return CaseResult{}, domain.ErrInvalidTransition
	}
	if ctx.Actor != data.Case.Assignee {
		return CaseResult{}, domain.ErrForbidden
	}
	version := len(data.Actions) + 1
	if len(data.Actions) > 0 {
		previous := data.Actions[len(data.Actions)-1]
		if err := domain.ValidateIssueResolutions(previous, cmd.IssueResolutions); err != nil {
			return CaseResult{}, err
		}
	}
	due := data.Case.DueAt.UTC()
	deadlineStatus, overdueMinutes := domain.DeadlineSubmittedOnTime, int64(0)
	if ctx.Now.After(due) {
		deadlineStatus, overdueMinutes = domain.DeadlineSubmittedLate, roundedUpMinutes(ctx.Now.Sub(due))
	}
	action := domain.CorrectiveAction{
		ID: newID("action"), TransitCaseID: cmd.CaseID, RootCause: data.Investigation.RootCause, ActionText: strings.TrimSpace(cmd.ActionText),
		CompletionNote: strings.TrimSpace(cmd.CompletionNote), Owner: ctx.Actor, DueAt: due, EvidenceRefs: cleanRefs(cmd.EvidenceRefs),
		SubmissionNumber: version, SubmittedAt: ctx.Now, DeadlineStatus: deadlineStatus, OverdueMinutes: overdueMinutes,
		OverdueReason: strings.TrimSpace(cmd.OverdueReason), IssueResolutions: append([]domain.IssueResolution(nil), cmd.IssueResolutions...),
	}
	if version > 1 {
		action.PreviousVersion = version - 1
	}
	if err := domain.ValidateCorrectiveAction(action); err != nil {
		return CaseResult{}, err
	}
	next := data.Case
	event, err := next.Change(newID("evt"), "correction_submitted", domain.StatePendingVerification, ctx, action)
	if err != nil {
		return CaseResult{}, err
	}
	deadline := &domain.DeadlineProjection{Status: deadlineStatus, DueAt: due, SubmittedAt: timePointer(ctx.Now), OverdueMinutes: overdueMinutes}
	result := CaseResult{Case: next, Assessment: data.Assessment, Deadline: deadline}
	err = s.repo.Commit(next, cmd.ExpectedRevision, store.Mutation{Action: &action}, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, result)
	return result, err
}

func cleanRefs(refs []string) []string {
	result := make([]string, len(refs))
	for i, ref := range refs {
		result[i] = strings.TrimSpace(ref)
	}
	return result
}

func copyStringMap(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[strings.TrimSpace(key)] = strings.TrimSpace(value)
	}
	return result
}

func timePointer(t time.Time) *time.Time { return &t }
