package httpapi

import (
	"net/http"

	"specimen-transit-guard/internal/workflow"
)

type API struct{ service *workflow.Service }

func NewHandler(service *workflow.Service) http.Handler {
	a := &API{service: service}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", a.Health)
	mux.HandleFunc("POST /api/v1/transit-cases", a.RegisterCase)
	mux.HandleFunc("GET /api/v1/transit-cases/{id}", a.GetCase)
	mux.HandleFunc("POST /api/v1/transit-cases/{id}/evidence", a.ReceiveEvidence)
	mux.HandleFunc("POST /api/v1/transit-cases/{id}/assessment", a.AssessCase)
	mux.HandleFunc("POST /api/v1/transit-cases/{id}/investigation", a.InvestigateCase)
	mux.HandleFunc("POST /api/v1/transit-cases/{id}/corrective-actions", a.SubmitCorrectiveAction)
	mux.HandleFunc("POST /api/v1/transit-cases/{id}/verification", a.VerifyCorrectiveAction)
	mux.HandleFunc("POST /api/v1/transit-cases/{id}/close", a.CloseCase)
	mux.HandleFunc("GET /api/v1/transit-cases/{id}/audit", a.GetAudit)
	mux.HandleFunc("GET /api/v1/transit-cases/{id}/closure-summary", a.GetClosureSummary)
	return recoverMiddleware(accessMiddleware(mux))
}

func (a *API) Health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
