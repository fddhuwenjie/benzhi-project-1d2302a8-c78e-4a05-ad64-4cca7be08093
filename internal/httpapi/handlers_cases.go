package httpapi

import (
	"net/http"
	"strconv"

	"specimen-transit-guard/internal/workflow"
)

type registerRequest struct {
	ShipmentCode    string  `json:"shipment_code"`
	ContainerCode   string  `json:"container_code"`
	SampleCategory  string  `json:"sample_category"`
	TemperatureMinC float64 `json:"temperature_min_c"`
	TemperatureMaxC float64 `json:"temperature_max_c"`
}

func (a *API) RegisterCase(w http.ResponseWriter, r *http.Request) {
	var body registerRequest
	if err := decodeJSON(w, r, &body); err != nil {
		writeError(w, err)
		return
	}
	meta, err := metadata(r, false)
	if err != nil {
		writeError(w, err)
		return
	}
	result, err := a.service.RegisterContext(r.Context(), workflow.RegisterCommand{Metadata: meta, ShipmentCode: body.ShipmentCode, ContainerCode: body.ContainerCode,
		SampleCategory: body.SampleCategory, TemperatureMinC: body.TemperatureMinC, TemperatureMaxC: body.TemperatureMaxC,
		RequestFingerprint: workflow.RegistrationFingerprint(body.ShipmentCode, body.ContainerCode, body.SampleCategory, body.TemperatureMinC, body.TemperatureMaxC)})
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", revisionETag(result.Case.Revision))
	writeJSON(w, http.StatusCreated, result)
}

func (a *API) GetCase(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.Get(r.PathValue("id"))
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("ETag", revisionETag(result.Case.Revision))
	writeJSON(w, http.StatusOK, result)
}

func revisionETag(revision int64) string { return `"` + strconv.FormatInt(revision, 10) + `"` }
