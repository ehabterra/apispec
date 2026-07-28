// Package debugroutes registers operator endpoints the same way api does. They
// are real routes — they are simply not part of the documented API.
package debugroutes

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Stats struct {
	Goroutines int `json:"goroutines"`
}

func Register(r chi.Router) {
	r.Get("/debug/stats", dumpStats)
}

func dumpStats(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Stats{})
}
