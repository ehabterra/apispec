// Fixture: r.FormValue reads must not be emitted with the invalid OpenAPI
// parameter location `in: form` (issue #171). The location is ambiguous in Go —
// FormValue reads the URL query for GET and the urlencoded body for POST — so
// the resolver picks a valid shape from the HTTP method: GET form values become
// `in: query`, and POST form values become an application/x-www-form-urlencoded
// request body.
package main

import (
	"encoding/json"
	"net/http"
)

type Result struct {
	Query string `json:"query"`
}

// search reads a form value on a GET — resolves to an `in: query` parameter.
func search(w http.ResponseWriter, r *http.Request) {
	q := r.FormValue("query")
	_ = json.NewEncoder(w).Encode(Result{Query: q})
}

// submit reads form values on a POST — resolves to a form-urlencoded body.
func submit(w http.ResponseWriter, r *http.Request) {
	name := r.FormValue("name")
	email := r.FormValue("email")
	_ = json.NewEncoder(w).Encode(Result{Query: name + email})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /search", search)
	mux.HandleFunc("POST /submit", submit)
	_ = http.ListenAndServe(":8080", mux)
}
