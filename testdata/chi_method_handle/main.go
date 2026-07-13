package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// HealthStatus is the /health response body.
type HealthStatus struct {
	Status string `json:"status"`
	Uptime int64  `json:"uptime"`
}

// HealthHandler implements http.Handler and is registered via r.Method.
type HealthHandler struct{}

func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthStatus{Status: "ok"})
}

// MetricsHandler is an opaque http.Handler (promhttp.Handler() stand-in),
// registered via r.Handle.
type MetricsHandler struct{}

func (m *MetricsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("metrics"))
}

// Deps carries handlers as values, mirroring dependency-injected routers.
type Deps struct {
	Health  *HealthHandler
	Metrics http.Handler
}

// ServeLive is a plain func value registered via r.Get.
func ServeLive(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("live"))
}

// readyHandler is registered via r.MethodFunc.
func readyHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(HealthStatus{Status: "ready"})
}

// itemsHandler dispatches on r.Method and is registered verb-less via
// r.HandleFunc — it must split into one operation per verb.
func itemsHandler(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("items"))
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

// NewRouter registers the same kind of endpoint several ways.
func NewRouter(deps Deps) *chi.Mux {
	r := chi.NewRouter()

	r.Get("/live", ServeLive)                        // func value
	r.Method(http.MethodGet, "/health", deps.Health) // http.Handler via Method
	r.MethodFunc(http.MethodPost, "/ready", readyHandler)
	r.Handle("/metrics", deps.Metrics) // opaque http.Handler
	r.HandleFunc("/items", itemsHandler)

	return r
}

func main() {
	deps := Deps{Health: &HealthHandler{}, Metrics: &MetricsHandler{}}
	_ = http.ListenAndServe(":8080", NewRouter(deps))
}
