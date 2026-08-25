package store

import (
	"encoding/json"

	"specimen-transit-guard/internal/domain"
)

type snapshot struct {
	Cases          map[string]domain.TransitCase          `json:"cases"`
	ShipmentIndex  map[string]string                      `json:"shipment_index"`
	Readings       map[string][]domain.TemperatureReading `json:"readings"`
	Evidence       map[string]domain.HandoffEvidence      `json:"evidence"`
	Assessments    map[string]domain.DeviationAssessment  `json:"assessments"`
	Investigations map[string]domain.Investigation        `json:"investigations"`
	Actions        map[string][]domain.CorrectiveAction   `json:"actions"`
	Idempotency    map[string]IdempotencyRecord           `json:"idempotency"`
}

type IdempotencyRecord struct {
	Fingerprint string          `json:"fingerprint,omitempty"`
	Response    json.RawMessage `json:"response"`
}

func (r *IdempotencyRecord) UnmarshalJSON(raw []byte) error {
	type alias IdempotencyRecord
	var current alias
	if err := json.Unmarshal(raw, &current); err != nil {
		return err
	}
	if current.Response != nil || current.Fingerprint != "" {
		*r = IdempotencyRecord(current)
		return nil
	}
	r.Response = append(r.Response[:0], raw...)
	return nil
}

func emptySnapshot() snapshot {
	return snapshot{Cases: map[string]domain.TransitCase{}, ShipmentIndex: map[string]string{},
		Readings: map[string][]domain.TemperatureReading{}, Evidence: map[string]domain.HandoffEvidence{},
		Assessments: map[string]domain.DeviationAssessment{}, Investigations: map[string]domain.Investigation{},
		Actions: map[string][]domain.CorrectiveAction{}, Idempotency: map[string]IdempotencyRecord{}}
}

type Mutation struct {
	Readings      []domain.TemperatureReading
	Evidence      *domain.HandoffEvidence
	Assessment    *domain.DeviationAssessment
	Investigation *domain.Investigation
	Action        *domain.CorrectiveAction
	UpdateAction  bool
}

type CaseData struct {
	Case             domain.TransitCase          `json:"case"`
	Readings         []domain.TemperatureReading `json:"readings"`
	Evidence         *domain.HandoffEvidence     `json:"handoff_evidence,omitempty"`
	Assessment       *domain.DeviationAssessment `json:"assessment,omitempty"`
	Investigation    *domain.Investigation       `json:"investigation,omitempty"`
	Actions          []domain.CorrectiveAction   `json:"corrective_actions"`
	EvidenceProgress *domain.EvidenceProgress    `json:"evidence_progress,omitempty"`
	Deadline         *domain.DeadlineProjection  `json:"deadline,omitempty"`
}
