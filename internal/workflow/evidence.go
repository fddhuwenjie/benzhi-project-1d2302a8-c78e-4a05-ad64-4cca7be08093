package workflow

import (
	"fmt"
	"sort"
	"strings"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/store"
)

func (s *Service) ReceiveEvidence(cmd EvidenceCommand) (CaseResult, error) {
	if err := requireIdempotency(cmd.Metadata); err != nil {
		return CaseResult{}, err
	}
	ctx, err := s.context(cmd.Metadata)
	if err != nil {
		return CaseResult{}, err
	}
	scope := "evidence:" + cmd.CaseID
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
	if data.Case.State != domain.StateDraft {
		return CaseResult{}, domain.ErrInvalidTransition
	}
	if len(cmd.Readings) == 0 && strings.TrimSpace(cmd.DocumentRef) == "" && strings.TrimSpace(cmd.DigestSHA256) == "" {
		return CaseResult{}, &domain.FieldError{Field: "evidence", Message: "必须提交温度批次、交接文档或二者"}
	}
	evidence, err := mergeHandoff(data.Evidence, cmd, ctx)
	if err != nil {
		return CaseResult{}, err
	}
	newReadings, err := validateBatch(cmd.Readings, data.Readings, evidence, cmd.CaseID, ctx)
	if err != nil {
		return CaseResult{}, err
	}
	merged := append(append([]domain.TemperatureReading(nil), data.Readings...), newReadings...)
	sort.Slice(merged, func(i, j int) bool { return merged[i].RecordedAt.Before(merged[j].RecordedAt) })
	progress := domain.BuildEvidenceProgress(&evidence, merged, s.calculator.CoverageSlop())
	nextState := domain.StateDraft
	if progress.Ready {
		nextState = domain.StateEvidenceReady
	}
	next := data.Case
	event, err := next.Change(newID("evt"), "evidence_received", nextState, ctx, map[string]any{
		"source_batches": batchNames(newReadings), "new_reading_count": len(newReadings), "document_ref": evidence.DocumentRef, "progress": progress,
	})
	if err != nil {
		return CaseResult{}, err
	}
	result := CaseResult{Case: next, EvidenceProgress: &progress}
	err = s.repo.Commit(next, cmd.ExpectedRevision, store.Mutation{Readings: newReadings, Evidence: &evidence}, []domain.AuditEvent{event}, scope, cmd.IdempotencyKey, result)
	if err == nil {
		s.invalidateAuditCache(cmd.CaseID)
	}
	return result, err
}

func mergeHandoff(existing *domain.HandoffEvidence, cmd EvidenceCommand, ctx domain.ChangeContext) (domain.HandoffEvidence, error) {
	var evidence domain.HandoffEvidence
	if existing != nil {
		evidence = *existing
	}
	if evidence.TransitCaseID == "" {
		evidence.TransitCaseID = cmd.CaseID
	}
	if evidence.TransportStartedAt.IsZero() {
		evidence.TransportStartedAt = cmd.TransportStartedAt.UTC()
	}
	if evidence.TransportEndedAt.IsZero() {
		evidence.TransportEndedAt = cmd.TransportEndedAt.UTC()
	}
	if evidence.TransportStartedAt.IsZero() || !evidence.TransportEndedAt.After(evidence.TransportStartedAt) {
		return domain.HandoffEvidence{}, &domain.FieldError{Field: "transport_period", Message: "首次证据提交必须提供有效运输起止时间"}
	}
	if !cmd.TransportStartedAt.IsZero() && !cmd.TransportStartedAt.UTC().Equal(evidence.TransportStartedAt) ||
		!cmd.TransportEndedAt.IsZero() && !cmd.TransportEndedAt.UTC().Equal(evidence.TransportEndedAt) {
		return domain.HandoffEvidence{}, &domain.FieldError{Field: "transport_period", Message: "运输起止时间与既有证据不一致"}
	}
	documentSupplied := strings.TrimSpace(cmd.DocumentRef) != "" || strings.TrimSpace(cmd.DigestSHA256) != ""
	if documentSupplied {
		candidate := evidence
		candidate.DocumentRef = strings.TrimSpace(cmd.DocumentRef)
		candidate.DigestSHA256 = strings.ToLower(strings.TrimSpace(cmd.DigestSHA256))
		candidate.ReceivedAt = ctx.Now
		if err := domain.ValidateHandoff(candidate); err != nil {
			return domain.HandoffEvidence{}, err
		}
		if existing != nil && existing.DocumentRef != "" && (existing.DocumentRef != candidate.DocumentRef || existing.DigestSHA256 != candidate.DigestSHA256) {
			return domain.HandoffEvidence{}, &domain.FieldError{Field: "handoff_document", Message: "已接收的交接文档不可覆盖"}
		}
		evidence = candidate
	}
	return evidence, nil
}

func validateBatch(inputs []domain.ReadingInput, existing []domain.TemperatureReading, evidence domain.HandoffEvidence, caseID string, ctx domain.ChangeContext) ([]domain.TemperatureReading, error) {
	seenTimes := make(map[int64]bool, len(existing)+len(inputs))
	seenIdentity := make(map[string]bool, len(existing)+len(inputs))
	for _, reading := range existing {
		seenTimes[reading.RecordedAt.UnixNano()] = true
		seenIdentity[reading.SourceBatch+"\x00"+reading.SensorSerial+"\x00"+reading.RecordedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")] = true
	}
	readings := make([]domain.TemperatureReading, len(inputs))
	for i, input := range inputs {
		if err := domain.ValidateReading(input, evidence.TransportStartedAt, evidence.TransportEndedAt); err != nil {
			return nil, fmt.Errorf("readings[%d]: %w", i, err)
		}
		if i > 0 && !input.RecordedAt.After(inputs[i-1].RecordedAt) {
			return nil, &domain.FieldError{Field: "readings", Message: "批次内必须按 recorded_at 严格递增"}
		}
		sensor, batch := strings.TrimSpace(input.SensorSerial), strings.TrimSpace(input.SourceBatch)
		identity := batch + "\x00" + sensor + "\x00" + input.RecordedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
		if seenTimes[input.RecordedAt.UnixNano()] || seenIdentity[identity] {
			return nil, &domain.FieldError{Field: fmt.Sprintf("readings[%d].recorded_at", i), Message: "采集时间与既有或本批读数重复"}
		}
		seenTimes[input.RecordedAt.UnixNano()] = true
		seenIdentity[identity] = true
		readings[i] = domain.TemperatureReading{ID: newID("reading"), TransitCaseID: caseID, RecordedAt: input.RecordedAt.UTC(),
			TemperatureC: input.TemperatureC, SensorSerial: sensor, SourceBatch: batch, ReceivedAt: ctx.Now}
	}
	return readings, nil
}

func batchNames(readings []domain.TemperatureReading) []string {
	seen := map[string]bool{}
	result := []string{}
	for _, reading := range readings {
		if !seen[reading.SourceBatch] {
			seen[reading.SourceBatch] = true
			result = append(result, reading.SourceBatch)
		}
	}
	return result
}
