package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"specimen-transit-guard/internal/domain"
)

type Repository struct {
	mu           sync.RWMutex
	dir          string
	snapshotPath string
	eventPath    string
	data         snapshot
	caseCache    map[string]CaseData
}

func Open(dir string) (*Repository, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	r := &Repository{dir: dir, snapshotPath: filepath.Join(dir, "snapshot.json"), eventPath: filepath.Join(dir, "events.jsonl"), data: emptySnapshot(), caseCache: map[string]CaseData{}}
	if err := r.loadSnapshot(); err != nil {
		return nil, err
	}
	if err := r.validateEventLog(); err != nil {
		return nil, err
	}
	r.rebuildIndexes()
	return r, nil
}

func (r *Repository) GetCase(id string) (CaseData, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if cached, ok := r.caseCache[id]; ok {
		return cloneCaseData(cached), nil
	}
	c, ok := r.data.Cases[id]
	if !ok {
		return CaseData{}, domain.ErrNotFound
	}
	data := r.caseDataLocked(c)
	r.caseCache[id] = cloneCaseData(data)
	return data, nil
}

func (r *Repository) FindByShipment(code string) (CaseData, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.data.ShipmentIndex[code]
	if !ok {
		return CaseData{}, domain.ErrNotFound
	}
	return r.caseDataLocked(r.data.Cases[id]), nil
}

func (r *Repository) caseDataLocked(c domain.TransitCase) CaseData {
	d := CaseData{
		Case:     c,
		Readings: cloneReadings(r.data.Readings[c.ID]),
		Actions:  cloneActions(r.data.Actions[c.ID]),
	}
	sort.Slice(d.Readings, func(i, j int) bool { return d.Readings[i].RecordedAt.Before(d.Readings[j].RecordedAt) })
	if v, ok := r.data.Evidence[c.ID]; ok {
		copy := v
		d.Evidence = &copy
	}
	if v, ok := r.data.Assessments[c.ID]; ok {
		copy := cloneAssessment(v)
		d.Assessment = &copy
	}
	if v, ok := r.data.Investigations[c.ID]; ok {
		copy := cloneInvestigation(v)
		d.Investigation = &copy
	}
	return d
}

func (r *Repository) Idempotent(scope, key, fingerprint string, out any) (bool, error) {
	r.mu.RLock()
	raw, ok := r.data.Idempotency[scope+":"+key]
	r.mu.RUnlock()
	if !ok {
		return false, nil
	}
	if fingerprint != "" && raw.Fingerprint != "" && raw.Fingerprint != fingerprint {
		return true, domain.ErrIdempotencyPayload
	}
	if err := json.Unmarshal(raw.Response, out); err != nil {
		return false, fmt.Errorf("读取幂等结果: %w", err)
	}
	return true, nil
}

func (r *Repository) Create(c domain.TransitCase, events []domain.AuditEvent, scope, key, fingerprint string, result any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, exists := r.data.ShipmentIndex[c.ShipmentCode]; exists {
		existing := r.data.Cases[id]
		return &domain.DuplicateShipmentError{CaseID: id, Revision: existing.Revision}
	}
	if _, exists := r.data.Cases[c.ID]; exists {
		return domain.ErrConflict
	}
	next := cloneSnapshot(r.data)
	next.Cases[c.ID] = c
	next.ShipmentIndex[c.ShipmentCode] = c.ID
	if err := putIdempotency(&next, scope, key, fingerprint, result); err != nil {
		return err
	}
	if err := r.persistLocked(next, events); err != nil {
		return err
	}
	return nil
}

func (r *Repository) Commit(nextCase domain.TransitCase, expected int64, mutation Mutation, events []domain.AuditEvent, scope, key string, result any) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	current, ok := r.data.Cases[nextCase.ID]
	if !ok {
		return domain.ErrNotFound
	}
	if current.Revision != expected || nextCase.Revision <= current.Revision {
		return domain.ErrConflict
	}
	next := cloneSnapshot(r.data)
	next.Cases[nextCase.ID] = nextCase
	if len(mutation.Readings) > 0 {
		next.Readings[nextCase.ID] = append(next.Readings[nextCase.ID], mutation.Readings...)
	}
	if mutation.Evidence != nil {
		next.Evidence[nextCase.ID] = *mutation.Evidence
	}
	if mutation.Assessment != nil {
		next.Assessments[nextCase.ID] = *mutation.Assessment
	}
	if mutation.Investigation != nil {
		next.Investigations[nextCase.ID] = *mutation.Investigation
	}
	if mutation.Action != nil {
		if mutation.UpdateAction {
			updated := false
			for i := range next.Actions[nextCase.ID] {
				if next.Actions[nextCase.ID][i].ID == mutation.Action.ID {
					next.Actions[nextCase.ID][i] = *mutation.Action
					updated = true
					break
				}
			}
			if !updated {
				return domain.ErrConflict
			}
		} else {
			next.Actions[nextCase.ID] = append(next.Actions[nextCase.ID], *mutation.Action)
		}
	}
	if err := putIdempotency(&next, scope, key, "", result); err != nil {
		return err
	}
	if err := r.persistLocked(next, events); err != nil {
		return err
	}
	delete(r.caseCache, nextCase.ID)
	return nil
}

func putIdempotency(s *snapshot, scope, key, fingerprint string, result any) error {
	if key == "" {
		return nil
	}
	raw, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("序列化幂等结果: %w", err)
	}
	s.Idempotency[scope+":"+key] = IdempotencyRecord{Fingerprint: fingerprint, Response: raw}
	return nil
}

func cloneSnapshot(in snapshot) snapshot {
	raw, _ := json.Marshal(in)
	out := emptySnapshot()
	_ = json.Unmarshal(raw, &out)
	return out
}

// cloneCaseData returns a deep copy of a CaseData so that callers cannot mutate
// nested mutable fields (slices, maps, pointer-backed structs) shared with the
// repository cache or the persisted snapshot.
func cloneCaseData(in CaseData) CaseData {
	out := CaseData{
		Case:             in.Case,
		Readings:         cloneReadings(in.Readings),
		Actions:          cloneActions(in.Actions),
		EvidenceProgress: in.EvidenceProgress,
		Deadline:         in.Deadline,
	}
	if in.Evidence != nil {
		copy := *in.Evidence
		out.Evidence = &copy
	}
	if in.Assessment != nil {
		copy := cloneAssessment(*in.Assessment)
		out.Assessment = &copy
	}
	if in.Investigation != nil {
		copy := cloneInvestigation(*in.Investigation)
		out.Investigation = &copy
	}
	return out
}

func cloneReadings(in []domain.TemperatureReading) []domain.TemperatureReading {
	if in == nil {
		return nil
	}
	out := make([]domain.TemperatureReading, len(in))
	copy(out, in)
	return out
}

func cloneActions(in []domain.CorrectiveAction) []domain.CorrectiveAction {
	if in == nil {
		return nil
	}
	out := make([]domain.CorrectiveAction, len(in))
	for i, a := range in {
		out[i] = a
		out[i].EvidenceRefs = cloneStrings(a.EvidenceRefs)
		out[i].IssueResolutions = cloneIssueResolutions(a.IssueResolutions)
		out[i].VerificationIssues = cloneVerificationIssues(a.VerificationIssues)
	}
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneIssueResolutions(in []domain.IssueResolution) []domain.IssueResolution {
	if in == nil {
		return nil
	}
	out := make([]domain.IssueResolution, len(in))
	copy(out, in)
	return out
}

func cloneVerificationIssues(in []domain.VerificationIssue) []domain.VerificationIssue {
	if in == nil {
		return nil
	}
	out := make([]domain.VerificationIssue, len(in))
	copy(out, in)
	return out
}

func cloneAssessment(in domain.DeviationAssessment) domain.DeviationAssessment {
	out := in
	out.Triggers = cloneStrings(in.Triggers)
	out.TriggerDetails = cloneAssessmentTriggers(in.TriggerDetails)
	out.Excursions = cloneExcursions(in.Excursions)
	out.LowTemperature = cloneDirectionalStats(in.LowTemperature)
	out.HighTemperature = cloneDirectionalStats(in.HighTemperature)
	out.MissingWindows = cloneMissingWindows(in.MissingWindows)
	return out
}

func cloneAssessmentTriggers(in []domain.AssessmentTrigger) []domain.AssessmentTrigger {
	if in == nil {
		return nil
	}
	out := make([]domain.AssessmentTrigger, len(in))
	copy(out, in)
	return out
}

func cloneExcursions(in []domain.Excursion) []domain.Excursion {
	if in == nil {
		return nil
	}
	out := make([]domain.Excursion, len(in))
	copy(out, in)
	return out
}

func cloneDirectionalStats(in domain.DirectionalExcursionStats) domain.DirectionalExcursionStats {
	out := in
	out.Intervals = cloneExcursions(in.Intervals)
	return out
}

func cloneMissingWindows(in []domain.MissingWindow) []domain.MissingWindow {
	if in == nil {
		return nil
	}
	out := make([]domain.MissingWindow, len(in))
	copy(out, in)
	return out
}

func cloneInvestigation(in domain.Investigation) domain.Investigation {
	out := in
	if in.TriggerImpacts != nil {
		out.TriggerImpacts = make(map[string]string, len(in.TriggerImpacts))
		for key, value := range in.TriggerImpacts {
			out.TriggerImpacts[key] = value
		}
	}
	return out
}

func (r *Repository) Audit(caseID string, offset, limit int) ([]domain.AuditEvent, int, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.data.Cases[caseID]; !ok {
		return nil, 0, domain.ErrNotFound
	}
	events, err := readEvents(r.eventPath)
	if err != nil {
		return nil, 0, err
	}
	filtered := events[:0]
	for _, e := range events {
		if e.TransitCaseID == caseID {
			filtered = append(filtered, e)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].OccurredAt.Equal(filtered[j].OccurredAt) {
			return filtered[i].Revision < filtered[j].Revision
		}
		return filtered[i].OccurredAt.Before(filtered[j].OccurredAt)
	})
	total := len(filtered)
	if offset >= total {
		return []domain.AuditEvent{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return append([]domain.AuditEvent(nil), filtered[offset:end]...), total, nil
}

func (r *Repository) AuditAll(caseID string) ([]domain.AuditEvent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.data.Cases[caseID]; !ok {
		return nil, domain.ErrNotFound
	}
	events, err := readEvents(r.eventPath)
	if err != nil {
		return nil, err
	}
	filtered := make([]domain.AuditEvent, 0)
	for _, event := range events {
		if event.TransitCaseID == caseID {
			filtered = append(filtered, event)
		}
	}
	sort.SliceStable(filtered, func(i, j int) bool { return filtered[i].Revision < filtered[j].Revision })
	return filtered, nil
}

func IsCorrupt(err error) bool { return errors.Is(err, errCorruptLog) }
