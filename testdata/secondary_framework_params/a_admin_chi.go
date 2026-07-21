// This file is named so it sorts BEFORE the gin API. Framework detection walks
// files in order and treats the first framework it sees as primary, so this
// small chi router makes gin the SECONDARY framework — the situation that used
// to strip gin's parameters, request bodies and responses (issue #211).
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Health struct {
	OK bool `json:"ok"`
}

func adminRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/admin/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Health{OK: true})
	})
	return r
}
