// Fixture: chi routers a helper creates and returns, with the caller
// registering on them (issue #633):
//
//	/api/inside       NewAPI registers on its own router, the caller mounts it
//	/api/outside      the caller registers on the returned router, then mounts it
//	/x                Sub mounts the router it returns itself — the caller's
//	                  route is not yet placed under that Mount (see the test)
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func h(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }

func NewAPI() chi.Router {
	m := chi.NewRouter()
	m.Get("/inside", h)
	return m
}

func Sub(parent chi.Router) chi.Router {
	s := chi.NewRouter()
	parent.Mount("/sub", s)
	return s
}

func main() {
	r := chi.NewRouter()

	api := NewAPI()
	api.Get("/outside", h)
	r.Mount("/api", api)

	s := Sub(r)
	s.Get("/x", h)

	_ = http.ListenAndServe(":8080", r)
}
