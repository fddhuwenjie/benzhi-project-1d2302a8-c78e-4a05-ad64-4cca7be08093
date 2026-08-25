package domain

import (
	"strings"
	"time"
)

type TemperatureReading struct {
	ID            string    `json:"id"`
	TransitCaseID string    `json:"transit_case_id"`
	RecordedAt    time.Time `json:"recorded_at"`
	TemperatureC  float64   `json:"temperature_c"`
	SensorSerial  string    `json:"sensor_serial"`
	SourceBatch   string    `json:"source_batch"`
	ReceivedAt    time.Time `json:"received_at"`
}

type ReadingInput struct {
	RecordedAt   time.Time `json:"recorded_at"`
	TemperatureC float64   `json:"temperature_c"`
	SensorSerial string    `json:"sensor_serial"`
	SourceBatch  string    `json:"source_batch"`
}

type HandoffEvidence struct {
	TransitCaseID      string    `json:"transit_case_id"`
	TransportStartedAt time.Time `json:"transport_started_at"`
	TransportEndedAt   time.Time `json:"transport_ended_at"`
	DocumentRef        string    `json:"document_ref"`
	DigestSHA256       string    `json:"digest_sha256"`
	ReceivedAt         time.Time `json:"received_at"`
}

type EvidenceProgress struct {
	HandoffReceived     bool       `json:"handoff_received"`
	TemperatureReceived bool       `json:"temperature_received"`
	ReadingCount        int        `json:"reading_count"`
	FirstRecordedAt     *time.Time `json:"first_recorded_at,omitempty"`
	LastRecordedAt      *time.Time `json:"last_recorded_at,omitempty"`
	MissingItems        []string   `json:"missing_items"`
	Ready               bool       `json:"ready"`
}

func ValidateReading(in ReadingInput, start, end time.Time) error {
	if in.RecordedAt.IsZero() || in.RecordedAt.Before(start) || in.RecordedAt.After(end) {
		return &FieldError{Field: "recorded_at", Message: "必须位于运输起止时间内"}
	}
	if in.TemperatureC < -150 || in.TemperatureC > 150 {
		return &FieldError{Field: "temperature_c", Message: "必须位于 -150 至 150 摄氏度"}
	}
	if strings.TrimSpace(in.SensorSerial) == "" || strings.TrimSpace(in.SourceBatch) == "" {
		return &FieldError{Field: "sensor_serial", Message: "传感器和来源批次不能为空"}
	}
	return nil
}

func ValidateHandoff(e HandoffEvidence) error {
	if e.TransportStartedAt.IsZero() || !e.TransportEndedAt.After(e.TransportStartedAt) {
		return &FieldError{Field: "transport_period", Message: "运输结束时间必须晚于开始时间"}
	}
	if strings.TrimSpace(e.DocumentRef) == "" {
		return &FieldError{Field: "document_ref", Message: "不能为空"}
	}
	if len(e.DigestSHA256) != 64 {
		return &FieldError{Field: "digest_sha256", Message: "必须是 64 位 SHA-256 摘要"}
	}
	for _, r := range e.DigestSHA256 {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return &FieldError{Field: "digest_sha256", Message: "必须为十六进制"}
		}
	}
	return nil
}

func BuildEvidenceProgress(e *HandoffEvidence, readings []TemperatureReading, coverageSlop time.Duration) EvidenceProgress {
	p := EvidenceProgress{MissingItems: []string{}, HandoffReceived: e != nil && strings.TrimSpace(e.DocumentRef) != "", TemperatureReceived: len(readings) > 0, ReadingCount: len(readings)}
	if len(readings) > 0 {
		first, last := readings[0].RecordedAt.UTC(), readings[len(readings)-1].RecordedAt.UTC()
		p.FirstRecordedAt, p.LastRecordedAt = &first, &last
	}
	if e == nil || e.TransportStartedAt.IsZero() || e.TransportEndedAt.IsZero() || len(readings) == 0 || readings[0].RecordedAt.Sub(e.TransportStartedAt) > coverageSlop {
		p.MissingItems = append(p.MissingItems, "start_coverage")
	}
	if e == nil || e.TransportStartedAt.IsZero() || e.TransportEndedAt.IsZero() || len(readings) == 0 || e.TransportEndedAt.Sub(readings[len(readings)-1].RecordedAt) > coverageSlop {
		p.MissingItems = append(p.MissingItems, "end_coverage")
	}
	if !p.HandoffReceived {
		p.MissingItems = append(p.MissingItems, "handoff_document")
	}
	p.Ready = len(readings) >= 2 && len(p.MissingItems) == 0
	return p
}
