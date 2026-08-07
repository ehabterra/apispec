// Fixture: the two shapes a name-based error test gets wrong, in one handler
// (issue #287).
//
//   - ErrorBudgetReport is the SUCCESS body. It is ordinary SLO domain data —
//     an error budget is a quantity, not a failure — and its name contains
//     "error" only incidentally. A substring test claims it as an error DTO and
//     demotes it, which drops a handler's real 200 body from the spec.
//   - ProblemDetails is the actual error body (RFC 7807). Its name contains
//     nothing an "error" test can see, so the same test does not recognise it.
//
// Both are exactly backwards, and both fail silently: the operation still has a
// response, it is just the wrong schema, with nothing to indicate a choice was
// made on a substring.
//
// MirrorConfig is the third case, and the cheapest to get wrong: "error" appears
// inside "MirrorConfig" as a plain substring, with no word boundary anywhere
// near it.
package main

import (
	"encoding/json"
	"net/http"
)

// ErrorBudgetReport is SLO data — the success payload of the report endpoint.
type ErrorBudgetReport struct {
	Service   string  `json:"service"`
	Budget    float64 `json:"budget"`
	Remaining float64 `json:"remaining"`
}

// ProblemDetails is RFC 7807, the genuine error body.
type ProblemDetails struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Status int    `json:"status"`
	Detail string `json:"detail"`
}

// MirrorConfig contains "error" as a substring and is not an error type.
type MirrorConfig struct {
	Upstream string `json:"upstream"`
	Enabled  bool   `json:"enabled"`
}

func budget(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("service") == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(ProblemDetails{Title: "service is required", Status: 400})
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(ErrorBudgetReport{Service: "api", Budget: 1, Remaining: 0.5})
}

func mirror(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(MirrorConfig{Upstream: "origin", Enabled: true})
}

func main() {
	http.HandleFunc("/budget", budget)
	http.HandleFunc("/mirror", mirror)
	_ = http.ListenAndServe(":8080", nil)
}
