// Fixture: a handler that answers with a literal nil sends `null`, and the spec
// must say so.
//
// `json.NewEncoder(w).Encode(nil)` writes exactly `null`. Documenting it as `{}`
// — the empty schema — says the response may be an object, an array, a string or
// anything else, which is the one thing this endpoint never does. The call site
// states the value; the spec should carry it (issue #404).
//
// `type: "null"` is JSON Schema 2020-12, which is what OpenAPI 3.1 is.
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID string `json:"id"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /items/{id}", getItem)
	mux.HandleFunc("DELETE /items/{id}", dropItem)
	http.ListenAndServe(":8080", mux)
}

// getItem answers the error branch with a literal nil and the success branch
// with a real body, so the two renderings sit side by side.
func getItem(w http.ResponseWriter, r *http.Request) {
	if r.PathValue("id") == "" {
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(nil)
		return
	}
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// dropItem answers nil on the success path: a null body, not an absent one.
func dropItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(nil)
}
