package domain

import (
	"encoding/json"
	"time"
)

type ChangeContext struct {
	Actor     string
	RequestID string
	Now       time.Time
}

type AuditEvent struct {
	EventID       string          `json:"event_id"`
	TransitCaseID string          `json:"transit_case_id"`
	EventType     string          `json:"event_type"`
	Actor         string          `json:"actor"`
	OccurredAt    time.Time       `json:"occurred_at"`
	RequestID     string          `json:"request_id"`
	FromState     State           `json:"from_state"`
	ToState       State           `json:"to_state"`
	Revision      int64           `json:"revision"`
	Payload       json.RawMessage `json:"payload,omitempty"`
}

func NewEvent(id, caseID, eventType string, ctx ChangeContext, from, to State, revision int64, payload any) AuditEvent {
	var raw json.RawMessage
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	return AuditEvent{EventID: id, TransitCaseID: caseID, EventType: eventType, Actor: ctx.Actor,
		OccurredAt: ctx.Now.UTC(), RequestID: ctx.RequestID, FromState: from, ToState: to,
		Revision: revision, Payload: raw}
}
