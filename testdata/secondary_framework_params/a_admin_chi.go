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

// adminSearch reads the two *http.Request patterns that HTTPSecondaryConfig
// gained here: FormValue and Cookie. The cookie read is the one that isolates
// the merge — chi's own config has a FormValue pattern but no Cookie pattern at
// all, so a resolved `session` cookie parameter can only have come from the
// layered net/http surface.
func adminSearch(w http.ResponseWriter, r *http.Request) {
	_ = r.FormValue("q")
	_, _ = r.Cookie("session")
	_ = json.NewEncoder(w).Encode(Health{OK: true})
}

func adminRouter() *chi.Mux {
	r := chi.NewRouter()
	r.Get("/admin/health", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(Health{OK: true})
	})
	r.Get("/admin/search", adminSearch)
	return r
}
