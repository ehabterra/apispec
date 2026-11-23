// Package sub builds a router in a local named like the caller's variable.
package sub

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func list(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

// Routes builds its router in `rt`, the very name its caller gives the result.
func Routes() *chi.Mux {
	rt := chi.NewRouter()
	rt.Get("/cross", list)
	return rt
}
