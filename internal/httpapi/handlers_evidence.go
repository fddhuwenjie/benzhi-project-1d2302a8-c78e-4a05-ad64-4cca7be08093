package httpapi

import (
	"net/http"
	"time"

	"specimen-transit-guard/internal/domain"
	"specimen-transit-guard/internal/workflow"
)

type evidenceRequest struct {
	TransportStartedAt time.Time             `json:"transport_started_at"`
	TransportEndedAt   time.Time             `json:"transport_ended_at"`
	DocumentRef        string                `json:"document_ref"`
	DigestSHA256       string                `json:"digest_sha256"`
	Readings           []domain.ReadingInput `json:"readings"`
}

func (a *API) ReceiveEvidence(w http.ResponseWriter, r *http.Request) {
	var body evidenceRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	meta, err := metadata(r, true)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.ReceiveEvidence(workflow.EvidenceCommand{Metadata: meta, CaseID: r.PathValue("id"),
		TransportStartedAt: body.TransportStartedAt, TransportEndedAt: body.TransportEndedAt, DocumentRef: body.DocumentRef,
		DigestSHA256: body.DigestSHA256, Readings: body.Readings})
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", revisionETag(result.Case.Revision))
	writeJSON(w, http.StatusOK, result)
}
