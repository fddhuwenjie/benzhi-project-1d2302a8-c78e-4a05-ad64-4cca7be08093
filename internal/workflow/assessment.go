package workflow

import (
	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func (s *Service) Assess(cmd AssessCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "assessment:" + cmd.CaseID
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
	if data.Case.State != domain.StateEvidenceReady || data.Evidence == nil {
		return CaseResult{}, domain.ErrInvalidTransition
	}
	assessmentResult, err := s.calculator.Evaluate(newID("assessment"), data.Case, *data.Evidence, data.Readings, ctx.Now)
	if err != nil {
		return CaseResult{}, err
	}
	nextState := domain.StateAssessmentPassed
	if assessmentResult.Result == domain.AssessmentInvestigate {
		nextState = domain.StatePendingInvestigation
	}
	next := data.Case
	event, err := next.Change(newID("evt"), "assessment_completed", nextState, ctx, assessmentResult)
	if err != nil {
		return CaseResult{}, err
	}
	result := CaseResult{Case: next, Assessment: &assessmentResult}
	err = s.repo.Commit(next, cmd.ExpectedRevision, store.Mutation{Assessment: &assessmentResult}, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, result)
	if err == nil {
		s.invalidateAuditCache(cmd.CaseID)
	}
	return result, err
}
