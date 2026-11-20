// A handler closure held in a local variable is the handler (issue #610),
// on plain net/http: both the short form and a declared http.HandlerFunc.
package main

import (
	"encoding/json"
	"net/http"
)

type Geo struct {
	Lat float64 `json:"lat"`
}

type Status struct {
	OK bool `json:"ok"`
}

// The closure reaches the registration through a helper's parameter: the
// parameter hop and the closure hop chain.
func register(mux *http.ServeMux, pattern string, h http.HandlerFunc) {
	mux.HandleFunc(pattern, h)
}

func main() {
	mux := http.NewServeMux()

	h := func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("count")
		json.NewEncoder(w).Encode(Geo{})
	}
	mux.HandleFunc("GET /geo", h)

	var health http.HandlerFunc = func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(Status{})
	}
	mux.Handle("GET /health", health)

	wrapped := func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("page")
		json.NewEncoder(w).Encode(Status{})
	}
	register(mux, "GET /wrapped", wrapped)

	// The control: inline.
	mux.HandleFunc("GET /inline", func(w http.ResponseWriter, r *http.Request) {
		_ = r.URL.Query().Get("count")
		json.NewEncoder(w).Encode(Geo{})
	})

	http.ListenAndServe(":8080", mux)
}
