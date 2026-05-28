// Package main exercises body arguments that are *call expressions*
// rather than identifiers. Without proper handling, the OpenAPI
// generator stringifies the call as "<receiver>.<method>" — e.g.
// "error.Error" — and emits an unresolvable $ref placeholder.
//
// All three routes should land on the actual *return type* of the
// call:
//
//   - GET  /errstr        → err.Error()        ⇒ string
//   - GET  /summary       → buildSummary(r)    ⇒ summary (named struct, $ref)
//   - GET  /count         → countItems()       ⇒ integer
package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

// summary is the concrete return type of buildSummary. It MUST appear
// under components/schemas — the /summary route's body schema is its
// $ref, not a stringified call.
type summary struct {
	Total  int    `json:"total"`
	Status string `json:"status"`
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/errstr", errstr)
	mux.HandleFunc("/summary", summarize)
	mux.HandleFunc("/count", count)
	_ = http.ListenAndServe(":8080", mux)
}

// errstr writes err.Error() as the response body. The body argument is
// a method-call expression whose return type is string.
func errstr(w http.ResponseWriter, r *http.Request) {
	err := errors.New("boom")
	http.Error(w, err.Error(), http.StatusBadRequest)
}

// summarize encodes a value produced by an in-line call. The body
// argument is a call expression returning a named struct (summary).
func summarize(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(buildSummary(r))
}

// count returns a plain integer produced by an in-line call.
func count(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(countItems())
}

func buildSummary(r *http.Request) summary {
	return summary{Total: 1, Status: "ok"}
}

func countItems() int {
	return 42
}
