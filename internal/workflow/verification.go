package workflow

import (
	"strings"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func (s *Service) Verify(cmd VerifyCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "verification:" + cmd.CaseID
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
	if len(data.Actions) == 0 {
		return CaseResult{}, domain.ErrInvalidTransition
	}
	last := data.Actions[len(data.Actions)-1]
	if data.Case.State != domain.StatePendingVerification {
		if last.VerificationResult != "" {
			return CaseResult{}, domain.ErrConflict
		}
		return CaseResult{}, domain.ErrInvalidTransition
	}
	if last.VerificationResult != "" {
		return CaseResult{}, domain.ErrConflict
	}
	if ctx.Actor == last.Owner || data.Investigation != nil && ctx.Actor == data.Investigation.ReviewedBy {
		return CaseResult{}, domain.ErrForbidden
	}
	if cmd.Accepted {
		if strings.TrimSpace(cmd.Note) == "" {
			return CaseResult{}, &domain.FieldError{Field: "note", Message: "接受整改时必须填写验证说明"}
		}
		if cmd.EvidenceVisible == nil || !*cmd.EvidenceVisible {
			return CaseResult{}, &domain.FieldError{Field: "evidence_visible", Message: "接受整改前必须确认全部证据引用可见"}
		}
	} else if err := validateVerificationIssues(cmd.Issues); err != nil {
		return CaseResult{}, err
	}
	verified := last
	verified.VerifiedBy, verified.VerificationNote = ctx.Actor, strings.TrimSpace(cmd.Note)
	verified.VerifiedAt = timePointer(ctx.Now)
	verified.VerificationIssues = append([]domain.VerificationIssue(nil), cmd.Issues...)
	to, resultText := domain.StatePendingCorrection, "rejected"
	if cmd.Accepted {
		to, resultText = domain.StateClosed, "accepted"
		verified.EvidenceVisible = true
	}
	verified.VerificationResult = resultText
	next := data.Case
	event, err := next.Change(newID("evt"), "correction_verified", to, ctx, verified)
	if err != nil {
		return CaseResult{}, err
	}
	result := CaseResult{Case: next, Assessment: data.Assessment}
	err = s.repo.Commit(next, cmd.ExpectedRevision, store.Mutation{Action: &verified, UpdateAction: true}, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, result)
	if err == nil {
		s.invalidateAuditCache(cmd.CaseID)
	}
	return result, err
}

func validateVerificationIssues(issues []domain.VerificationIssue) error {
	if len(issues) == 0 {
		return &domain.FieldError{Field: "issues", Message: "驳回整改时必须提供至少一项结构化问题"}
	}
	seen := map[string]bool{}
	for i, issue := range issues {
		if strings.TrimSpace(issue.ID) == "" || strings.TrimSpace(issue.Category) == "" || strings.TrimSpace(issue.Description) == "" {
			return &domain.FieldError{Field: "issues", Message: "每项问题的 id、category 和 description 均不能为空"}
		}
		if seen[issue.ID] {
			return &domain.FieldError{Field: "issues", Message: "问题 id 不能重复"}
		}
		seen[issue.ID] = true
		issues[i].ID = strings.TrimSpace(issue.ID)
	}
	return nil
}

func (s *Service) ClosePassed(cmd CloseCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "close:" + cmd.CaseID
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
	if data.Case.State != domain.StateAssessmentPassed {
		return CaseResult{}, domain.ErrInvalidTransition
	}
	if strings.TrimSpace(cmd.Reason) == "" {
		return CaseResult{}, &domain.FieldError{Field: "reason", Message: "关闭理由不能为空"}
	}
	next := data.Case
	event, err := next.Change(newID("evt"), "case_closed", domain.StateClosed, ctx, map[string]string{"reason": strings.TrimSpace(cmd.Reason)})
	if err != nil {
		return CaseResult{}, err
	}
	result := CaseResult{Case: next, Assessment: data.Assessment}
	err = s.repo.Commit(next, cmd.ExpectedRevision, store.Mutation{}, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, result)
	if err == nil {
		s.invalidateAuditCache(cmd.CaseID)
	}
	return result, err
}
