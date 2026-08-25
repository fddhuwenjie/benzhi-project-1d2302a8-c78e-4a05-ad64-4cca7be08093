package workflow

import (
	"encoding/json"
	"fmt"

	"specimen-transit-guard/internal/domain"
)

type AuditPage struct {
	Items  []domain.AuditEvent `json:"items"`
	Offset int                 `json:"offset"`
	Limit  int                 `json:"limit"`
	Total  int                 `json:"total"`
}

type auditCacheKey struct {
	caseID string
	offset int
	limit  int
}

type EvidenceCatalogItem struct {
	Reference string `json:"reference"`
	Kind      string `json:"kind"`
	Stage     string `json:"stage"`
	Revision  int64  `json:"revision"`
}

type CompletenessStage struct {
	Stage    string `json:"stage"`
	Complete bool   `json:"complete"`
	Revision int64  `json:"revision,omitempty"`
	Issue    string `json:"issue,omitempty"`
}

type ClosureSummary struct {
	Case                domain.TransitCase          `json:"case"`
	Assessment          *domain.DeviationAssessment `json:"assessment,omitempty"`
	Evidence            *domain.HandoffEvidence     `json:"handoff_evidence,omitempty"`
	ReadingCount        int                         `json:"reading_count"`
	RawEvidenceRefs     []string                    `json:"raw_evidence_refs"`
	EvidenceCatalog     []EvidenceCatalogItem       `json:"evidence_catalog"`
	Investigation       *domain.Investigation       `json:"investigation,omitempty"`
	CorrectiveActions   []domain.CorrectiveAction   `json:"corrective_actions"`
	ClosedEvent         *domain.AuditEvent          `json:"closed_event,omitempty"`
	Completeness        []CompletenessStage         `json:"completeness_checklist"`
	RevisionsContinuous bool                        `json:"revisions_continuous"`
	RuleVersion         string                      `json:"rule_version"`
	FinalDisposition    string                      `json:"final_disposition"`
}

func (s *Service) Audit(caseID string, offset, limit int) (AuditPage, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	key := auditCacheKey{caseID: caseID, offset: offset, limit: limit}
	s.auditMu.RLock()
	cached, ok := s.auditCache[key]
	s.auditMu.RUnlock()
	if ok {
		return cloneAuditPage(cached), nil
	}
	items, total, err := s.repo.Audit(caseID, offset, limit)
	if err != nil {
		return AuditPage{}, err
	}
	page := AuditPage{Items: items, Offset: offset, Limit: limit, Total: total}
	s.auditMu.Lock()
	s.auditCache[key] = cloneAuditPage(page)
	s.auditMu.Unlock()
	return page, nil
}

func cloneAuditPage(page AuditPage) AuditPage {
	result := page
	result.Items = make([]domain.AuditEvent, len(page.Items))
	for i, event := range page.Items {
		result.Items[i] = event
		result.Items[i].Payload = append([]byte(nil), event.Payload...)
	}
	return result
}

func (s *Service) Summary(caseID string) (ClosureSummary, error) {
	data, err := s.repo.GetCase(caseID)
	if err != nil {
		return ClosureSummary{}, err
	}
	if data.Case.State != domain.StateClosed {
		return ClosureSummary{}, domain.ErrInvalidTransition
	}
	events, err := s.repo.AuditAll(caseID)
	if err != nil {
		return ClosureSummary{}, err
	}
	checklist, missing, continuous, closed := validateClosureFlow(data.Case, data.Assessment != nil && data.Assessment.Result == domain.AssessmentPass, data.Actions, events)
	if data.Evidence == nil || data.Evidence.DocumentRef == "" {
		missing = append(missing, "handoff_document")
	}
	if len(data.Readings) == 0 {
		missing = append(missing, "temperature_readings")
	}
	if data.Assessment == nil {
		missing = append(missing, "assessment_projection")
	} else if data.Assessment.ID == "" || data.Assessment.RuleVersion == "" {
		missing = append(missing, "assessment_identity_or_rule_version")
	}
	if data.Assessment != nil && data.Assessment.Result == domain.AssessmentInvestigate && data.Investigation == nil {
		missing = append(missing, "investigation_projection")
	}
	if data.Investigation != nil && data.Investigation.NeedsCorrection && len(data.Actions) == 0 {
		missing = append(missing, "corrective_action_versions")
	}
	if data.Investigation != nil && !data.Investigation.NeedsCorrection && len(data.Actions) > 0 {
		missing = append(missing, "unexpected_corrective_action_versions")
	}
	catalog, refs := buildEvidenceCatalog(data.Evidence, data.Readings, data.Actions, events)
	for _, item := range catalog {
		if item.Revision == 0 {
			missing = append(missing, "unlinked_evidence:"+item.Kind+":"+item.Reference)
		}
	}
	if len(missing) > 0 {
		return ClosureSummary{}, &domain.SummaryIncompleteError{MissingItems: uniqueStrings(missing)}
	}
	disposition := "assessment_passed"
	if data.Investigation != nil {
		disposition = data.Investigation.Disposition
	}
	ruleVersion := ""
	if data.Assessment != nil {
		ruleVersion = data.Assessment.RuleVersion
	}
	return ClosureSummary{
		Case: data.Case, Assessment: data.Assessment, Evidence: data.Evidence, ReadingCount: len(data.Readings), RawEvidenceRefs: refs,
		EvidenceCatalog: catalog, Investigation: data.Investigation, CorrectiveActions: data.Actions, ClosedEvent: closed,
		Completeness: checklist, RevisionsContinuous: continuous, RuleVersion: ruleVersion, FinalDisposition: disposition,
	}, nil
}

func validateClosureFlow(tc domain.TransitCase, assessmentPassed bool, actions []domain.CorrectiveAction, events []domain.AuditEvent) ([]CompletenessStage, []string, bool, *domain.AuditEvent) {
	missing := []string{}
	continuous := len(events) == int(tc.Revision)
	for i, event := range events {
		if event.Revision != int64(i+1) {
			continuous = false
			missing = append(missing, fmt.Sprintf("revision_sequence_at_%d", i+1))
		}
		if i > 0 && event.FromState != events[i-1].ToState {
			missing = append(missing, fmt.Sprintf("audit_state_sequence_at_%d", event.Revision))
		}
	}
	if !continuous {
		missing = append(missing, "audit_revision_continuity")
	}
	stageRevision := map[string]int64{}
	correctionRevisions, verificationRevisions := []int64{}, []int64{}
	var closed *domain.AuditEvent
	for i := range events {
		event := events[i]
		switch event.EventType {
		case "case_registered":
			stageRevision["registration"] = event.Revision
		case "evidence_received":
			if event.ToState == domain.StateEvidenceReady {
				stageRevision["evidence_ready"] = event.Revision
			}
		case "assessment_completed":
			stageRevision["assessment"] = event.Revision
		case "investigation_completed":
			stageRevision["investigation"] = event.Revision
		case "correction_submitted":
			correctionRevisions = append(correctionRevisions, event.Revision)
		case "correction_verified":
			verificationRevisions = append(verificationRevisions, event.Revision)
		}
		if event.ToState == domain.StateClosed {
			copy := event
			closed = &copy
		}
	}
	checklist := []CompletenessStage{}
	appendStage := func(stage string, rev int64) {
		item := CompletenessStage{Stage: stage, Complete: rev > 0, Revision: rev}
		if rev == 0 {
			item.Issue = "缺少阶段事件"
			missing = append(missing, stage)
		}
		checklist = append(checklist, item)
	}
	appendStage("registration", stageRevision["registration"])
	appendStage("evidence_ready", stageRevision["evidence_ready"])
	appendStage("assessment", stageRevision["assessment"])
	if !assessmentPassed {
		appendStage("investigation", stageRevision["investigation"])
	} else {
		appendStage("direct_pass", stageRevision["assessment"])
	}
	if stageRevision["registration"] >= stageRevision["evidence_ready"] || stageRevision["evidence_ready"] >= stageRevision["assessment"] {
		missing = append(missing, "core_stage_order")
	}
	if !assessmentPassed && stageRevision["assessment"] >= stageRevision["investigation"] {
		missing = append(missing, "investigation_stage_order")
	}
	if len(correctionRevisions) != len(actions) || len(verificationRevisions) != len(actions) {
		missing = append(missing, "correction_verification_stage_count")
	}
	for i, action := range actions {
		var correction, verification int64
		if i < len(correctionRevisions) {
			correction = correctionRevisions[i]
		}
		if i < len(verificationRevisions) {
			verification = verificationRevisions[i]
		}
		appendStage(fmt.Sprintf("correction_v%d", i+1), correction)
		appendStage(fmt.Sprintf("verification_v%d", i+1), verification)
		if correction > 0 && verification > 0 && verification <= correction {
			missing = append(missing, fmt.Sprintf("correction_v%d_order", i+1))
		}
		if action.SubmissionNumber != i+1 || i > 0 && action.PreviousVersion != i {
			missing = append(missing, fmt.Sprintf("correction_v%d_version_link", i+1))
		}
		if action.VerificationResult != "accepted" && action.VerificationResult != "rejected" || action.VerifiedAt == nil || action.VerifiedBy == "" {
			missing = append(missing, fmt.Sprintf("verification_v%d_decision", i+1))
		}
		if i < len(actions)-1 && action.VerificationResult != "rejected" {
			missing = append(missing, fmt.Sprintf("verification_v%d_expected_rejection", i+1))
		}
		if i == len(actions)-1 && action.VerificationResult != "accepted" {
			missing = append(missing, fmt.Sprintf("verification_v%d_expected_acceptance", i+1))
		}
		if i > 0 && i-1 < len(verificationRevisions) && correction > 0 && correction <= verificationRevisions[i-1] {
			missing = append(missing, fmt.Sprintf("correction_v%d_previous_decision_order", i+1))
		}
	}
	closedRevision := int64(0)
	if closed != nil {
		closedRevision = closed.Revision
	}
	appendStage("closed", closedRevision)
	if closed == nil || closed.Revision != tc.Revision || len(events) > 0 && events[len(events)-1].ToState != tc.State {
		missing = append(missing, "final_closed_revision")
	}
	return checklist, missing, continuous, closed
}

func buildEvidenceCatalog(evidence *domain.HandoffEvidence, readings []domain.TemperatureReading, actions []domain.CorrectiveAction, events []domain.AuditEvent) ([]EvidenceCatalogItem, []string) {
	evidenceRevision, batches := int64(0), map[string]int64{}
	actionRevisions := map[string]int64{}
	for _, event := range events {
		if event.EventType == "evidence_received" {
			var payload struct {
				SourceBatches []string `json:"source_batches"`
				DocumentRef   string   `json:"document_ref"`
			}
			_ = json.Unmarshal(event.Payload, &payload)
			if payload.DocumentRef != "" && evidenceRevision == 0 {
				evidenceRevision = event.Revision
			}
			for _, batch := range payload.SourceBatches {
				if batches[batch] == 0 {
					batches[batch] = event.Revision
				}
			}
		}
		if event.EventType == "correction_submitted" {
			var action domain.CorrectiveAction
			_ = json.Unmarshal(event.Payload, &action)
			actionRevisions[action.ID] = event.Revision
		}
	}
	catalog := []EvidenceCatalogItem{}
	refs := []string{}
	seen := map[string]bool{}
	add := func(ref, kind, stage string, revision int64) {
		key := kind + "\x00" + ref
		if ref == "" || seen[key] {
			return
		}
		seen[key] = true
		catalog = append(catalog, EvidenceCatalogItem{Reference: ref, Kind: kind, Stage: stage, Revision: revision})
		refs = append(refs, ref)
	}
	if evidence != nil {
		add(evidence.DocumentRef, "handoff_document", "evidence", evidenceRevision)
	}
	for _, reading := range readings {
		add(reading.SourceBatch, "temperature_batch", "evidence", batches[reading.SourceBatch])
	}
	for _, action := range actions {
		for _, ref := range action.EvidenceRefs {
			add(ref, "corrective_evidence", fmt.Sprintf("correction_v%d", action.SubmissionNumber), actionRevisions[action.ID])
		}
	}
	return catalog, refs
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}
