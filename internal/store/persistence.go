package store

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"specimen-transit-guard/internal/domain"
)

var errCorruptLog = errors.New("审计日志损坏")

func (r *Repository) loadSnapshot() error {
	raw, err := os.ReadFile(r.snapshotPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取快照: %w", err)
	}
	if err := json.Unmarshal(raw, &r.data); err != nil {
		return fmt.Errorf("解析快照: %w", err)
	}
	return nil
}

func (r *Repository) rebuildIndexes() {
	if r.data.Cases == nil {
		r.data.Cases = map[string]domain.TransitCase{}
	}
	if r.data.ShipmentIndex == nil {
		r.data.ShipmentIndex = map[string]string{}
	}
	for id, c := range r.data.Cases {
		r.data.ShipmentIndex[c.ShipmentCode] = id
	}
	if r.data.Idempotency == nil {
		r.data.Idempotency = map[string]IdempotencyRecord{}
	}
	if r.data.Readings == nil {
		r.data.Readings = map[string][]domain.TemperatureReading{}
	}
	if r.data.Evidence == nil {
		r.data.Evidence = map[string]domain.HandoffEvidence{}
	}
	if r.data.Assessments == nil {
		r.data.Assessments = map[string]domain.DeviationAssessment{}
	}
	if r.data.Investigations == nil {
		r.data.Investigations = map[string]domain.Investigation{}
	}
	if r.data.Actions == nil {
		r.data.Actions = map[string][]domain.CorrectiveAction{}
	}
}

func (r *Repository) persistLocked(next snapshot, events []domain.AuditEvent) error {
	if err := atomicSnapshot(r.dir, r.snapshotPath, next); err != nil {
		return err
	}
	if err := appendEvents(r.eventPath, events); err != nil {
		r.data = next
		return err
	}
	r.data = next
	return nil
}

func appendEvents(path string, events []domain.AuditEvent) error {
	if len(events) == 0 {
		return nil
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o640)
	if err != nil {
		return fmt.Errorf("打开审计日志: %w", err)
	}
	w := bufio.NewWriter(f)
	for _, event := range events {
		raw, err := json.Marshal(event)
		if err != nil {
			f.Close()
			return err
		}
		if _, err := w.Write(append(raw, '\n')); err != nil {
			f.Close()
			return err
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func atomicSnapshot(dir, path string, data snapshot) error {
	raw, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化快照: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".snapshot-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if err := tmp.Chmod(0o640); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("替换快照: %w", err)
	}
	return nil
}

func (r *Repository) validateEventLog() error {
	_, err := readEvents(r.eventPath)
	return err
}

func readEvents(path string) ([]domain.AuditEvent, error) {
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return []domain.AuditEvent{}, nil
	}
	if err != nil {
		return nil, err
	}
	if len(raw) > 0 && raw[len(raw)-1] != '\n' {
		return nil, fmt.Errorf("%w: 末行被截断", errCorruptLog)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	events := []domain.AuditEvent{}
	for {
		var event domain.AuditEvent
		err := dec.Decode(&event)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil || event.EventID == "" || event.TransitCaseID == "" {
			return nil, fmt.Errorf("%w: %v", errCorruptLog, err)
		}
		events = append(events, event)
	}
	return events, nil
}
