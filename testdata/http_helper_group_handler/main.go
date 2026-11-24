// The net/http control for gin_helper_group_handler (issue #615): a helper that
// receives both the mux and the handler, registering on the root mux and on a
// sub-mux mounted under a prefix.
package main

import (
	"encoding/json"
	"net/http"
)

// Place is the body every handler in this fixture returns.
type Place struct {
	Name string `json:"name"`
}

func named(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("page")
	_ = json.NewEncoder(w).Encode(Place{})
}

func handA(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("page")
	_ = json.NewEncoder(w).Encode(Place{})
}

func handB(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("page")
	_ = json.NewEncoder(w).Encode(Place{})
}

func logged(w http.ResponseWriter, r *http.Request) {
	_ = r.URL.Query().Get("page")
	_ = json.NewEncoder(w).Encode(Place{})
}

func withLogging(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) { next(w, r) }
}

func reg(mux *http.ServeMux, h http.HandlerFunc) { mux.HandleFunc("GET /named", h) }

// The parameter is reassigned to a wrapped copy of itself: the handler is still
// the one the caller passed, peeled out of the middleware.
func regLogged(mux *http.ServeMux, h http.HandlerFunc) {
	h = withLogging(h)
	mux.HandleFunc("GET /logged", h)
}

func register(mux *http.ServeMux, pattern string, h http.HandlerFunc) { mux.HandleFunc(pattern, h) }

func main() {
	mux := http.NewServeMux()
	reg(mux, named)
	regLogged(mux, logged)

	api := http.NewServeMux()
	register(api, "GET /a", handA)
	register(api, "GET /b", handB)
	mux.Handle("/api/", http.StripPrefix("/api", api))

	_ = http.ListenAndServe(":8080", mux)
}
