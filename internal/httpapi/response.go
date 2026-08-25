package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"specimen-transit-guard/internal/domain"
)

type errorEnvelope struct {
	Error apiError `json:"error"`
}

type apiError struct {
	Code            string   `json:"code"`
	Message         string   `json:"message"`
	Field           string   `json:"field,omitempty"`
	ExistingCaseID  string   `json:"existing_case_id,omitempty"`
	CurrentRevision int64    `json:"current_revision,omitempty"`
	MissingItems    []string `json:"missing_items,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status, code, message := http.StatusInternalServerError, "internal_error", "服务内部错误"
	var fieldErr *domain.FieldError
	var consistencyErr *domain.InvestigationConsistencyError
	switch {
	case errors.As(err, &consistencyErr):
		status, code, message = http.StatusUnprocessableEntity, "investigation_inconsistent", consistencyErr.Message
	case errors.As(err, &fieldErr):
		status, code, message = http.StatusBadRequest, "validation_failed", fieldErr.Message
	case errors.Is(err, domain.ErrInvalid), errors.Is(err, domain.ErrEvidenceIncomplete):
		status, code, message = http.StatusUnprocessableEntity, "invalid_evidence", err.Error()
	case errors.Is(err, domain.ErrNotFound):
		status, code, message = http.StatusNotFound, "not_found", "运输任务不存在"
	case errors.Is(err, domain.ErrConflict), errors.Is(err, domain.ErrDuplicate):
		status, code, message = http.StatusConflict, "conflict", err.Error()
	case errors.Is(err, domain.ErrIdempotencyPayload):
		status, code, message = http.StatusConflict, "idempotency_payload_conflict", err.Error()
	case errors.Is(err, domain.ErrSummaryIncomplete):
		status, code, message = http.StatusConflict, "closure_summary_incomplete", err.Error()
	case errors.Is(err, domain.ErrInvalidTransition):
		status, code, message = http.StatusConflict, "invalid_state", err.Error()
	case errors.Is(err, domain.ErrForbidden):
		status, code, message = http.StatusForbidden, "forbidden", err.Error()
	}
	e := apiError{Code: code, Message: message}
	if fieldErr != nil {
		e.Field = fieldErr.Field
	}
	if consistencyErr != nil {
		e.Field = consistencyErr.Field
	}
	var duplicate *domain.DuplicateShipmentError
	if errors.As(err, &duplicate) {
		e.Code, e.ExistingCaseID, e.CurrentRevision = "duplicate_shipment", duplicate.CaseID, duplicate.Revision
	}
	var incomplete *domain.SummaryIncompleteError
	if errors.As(err, &incomplete) {
		e.MissingItems = append([]string(nil), incomplete.MissingItems...)
	}
	writeJSON(w, status, errorEnvelope{Error: e})
}
