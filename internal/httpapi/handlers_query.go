package httpapi

import (
	"net/http"

	"specimen-transit-guard/internal/domain"
)

func (a *API) GetAudit(w http.ResponseWriter, r *http.Request) {
	offset, err := queryInt(r, "offset", 0)
	if err != nil {
		writeError(w, &domain.FieldError{Field: "offset", Message: err.Error()})
		return
	}
	limit, err := queryInt(r, "limit", 50)
	if err != nil {
		writeError(w, &domain.FieldError{Field: "limit", Message: err.Error()})
		return
	}
	page, err := a.service.Audit(r.PathValue("id"), offset, limit)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, page)
}

func (a *API) GetClosureSummary(w http.ResponseWriter, r *http.Request) {
	summary, err := a.service.Summary(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, summary)
}
