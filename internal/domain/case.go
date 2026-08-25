package domain

import (
	"math"
	"strings"
	"time"
)

type TransitCase struct {
	ID              string     `json:"id"`
	ShipmentCode    string     `json:"shipment_code"`
	ContainerCode   string     `json:"container_code"`
	SampleCategory  string     `json:"sample_category"`
	TemperatureMinC float64    `json:"temperature_min_c"`
	TemperatureMaxC float64    `json:"temperature_max_c"`
	State           State      `json:"state"`
	Assignee        string     `json:"assignee,omitempty"`
	DueAt           *time.Time `json:"due_at,omitempty"`
	Revision        int64      `json:"revision"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type NewCaseInput struct {
	ShipmentCode, ContainerCode, SampleCategory string
	TemperatureMinC, TemperatureMaxC            float64
}

func NewTransitCase(id string, in NewCaseInput, ctx ChangeContext, eventID string) (TransitCase, AuditEvent, error) {
	if strings.TrimSpace(in.ShipmentCode) == "" {
		return TransitCase{}, AuditEvent{}, &FieldError{Field: "shipment_code", Message: "不能为空"}
	}
	if strings.TrimSpace(in.ContainerCode) == "" {
		return TransitCase{}, AuditEvent{}, &FieldError{Field: "container_code", Message: "不能为空"}
	}
	if strings.TrimSpace(in.SampleCategory) == "" {
		return TransitCase{}, AuditEvent{}, &FieldError{Field: "sample_category", Message: "不能为空"}
	}
	if math.IsNaN(in.TemperatureMinC) || math.IsNaN(in.TemperatureMaxC) || math.IsInf(in.TemperatureMinC, 0) || math.IsInf(in.TemperatureMaxC, 0) ||
		in.TemperatureMinC < -100 || in.TemperatureMaxC > 100 || in.TemperatureMinC >= in.TemperatureMaxC {
		return TransitCase{}, AuditEvent{}, &FieldError{Field: "temperature_range", Message: "必须位于 -100 至 100 摄氏度且下限小于上限"}
	}
	now := ctx.Now.UTC()
	c := TransitCase{ID: id, ShipmentCode: strings.ToUpper(strings.TrimSpace(in.ShipmentCode)), ContainerCode: strings.ToUpper(strings.TrimSpace(in.ContainerCode)),
		SampleCategory: strings.ToLower(strings.TrimSpace(in.SampleCategory)), TemperatureMinC: in.TemperatureMinC,
		TemperatureMaxC: in.TemperatureMaxC, State: StateDraft, Revision: 1, CreatedAt: now, UpdatedAt: now}
	e := NewEvent(eventID, id, "case_registered", ctx, "", StateDraft, c.Revision, map[string]any{"shipment_code": c.ShipmentCode})
	return c, e, nil
}

func (c *TransitCase) Change(eventID, eventType string, to State, ctx ChangeContext, payload any) (AuditEvent, error) {
	if ctx.Actor == "" {
		return AuditEvent{}, &FieldError{Field: "actor", Message: "不能为空"}
	}
	from := c.State
	if to != from && !CanTransition(from, to) {
		return AuditEvent{}, ErrInvalidTransition
	}
	c.State = to
	c.Revision++
	c.UpdatedAt = ctx.Now.UTC()
	return NewEvent(eventID, c.ID, eventType, ctx, from, to, c.Revision, payload), nil
}
