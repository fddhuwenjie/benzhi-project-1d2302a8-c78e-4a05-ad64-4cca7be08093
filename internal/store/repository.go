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
}

func Open(dir string) (*Repository, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("创建数据目录: %w", err)
	}
	r := &Repository{dir: dir, snapshotPath: filepath.Join(dir, "snapshot.json"), eventPath: filepath.Join(dir, "events.jsonl"), data: emptySnapshot()}
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
	r.mu.RLock()
	defer r.mu.RUnlock()
	c, ok := r.data.Cases[id]
	if !ok {
		return CaseData{}, domain.ErrNotFound
	}
	return r.caseDataLocked(c), nil
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
	d := CaseData{Case: c, Readings: append([]domain.TemperatureReading(nil), r.data.Readings[c.ID]...), Actions: append([]domain.CorrectiveAction(nil), r.data.Actions[c.ID]...)}
	sort.Slice(d.Readings, func(i, j int) bool { return d.Readings[i].RecordedAt.Before(d.Readings[j].RecordedAt) })
	if v, ok := r.data.Evidence[c.ID]; ok {
		copy := v
		d.Evidence = &copy
	}
	if v, ok := r.data.Assessments[c.ID]; ok {
		copy := v
		d.Assessment = &copy
	}
	if v, ok := r.data.Investigations[c.ID]; ok {
		copy := v
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
	return r.persistLocked(next, events)
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
	return r.persistLocked(next, events)
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
