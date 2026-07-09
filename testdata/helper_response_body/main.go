// Package main reproduces a bug seen in real codebases:
// when two or more handlers write their response through the *same*
// indirection helper — writeJSON(w, status, v any) — the per-route
// parameter trace from the helper's `v` back to the caller's concrete
// argument fails for at least one of the handlers, and its response
// schema collapses to a bare `type: object`.
//
// All three routes call writeJSON with the same shape:
//
//	out, err := h.repo.List(...)         // []items.Item
//	writeJSON(w, http.StatusOK, out)
//
// All three response schemas MUST therefore be `array of $ref(Item)`.
// The bug appears as one or more routes resolving to a bare
// `type: object` instead — observed at /c with the current ordering.
package main

import (
	"encoding/json"
	"net/http"

	"testdata/helper_response_body/items"
)

// writeJSON is the indirection helper. Its third parameter is `any`,
// so APISpec must parameter-trace each handler's call site back to
// the caller's concrete argument to recover the response schema.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type handlerA struct{ repo *items.Repo }

func (h *handlerA) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.repo.List(r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type handlerB struct{ repo *items.Repo }

func (h *handlerB) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.repo.List(r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

type handlerC struct{ repo *items.Repo }

func (h *handlerC) list(w http.ResponseWriter, r *http.Request) {
	out, err := h.repo.List(r.URL.Query().Get("q"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, out)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/a", (&handlerA{repo: items.New()}).list)
	mux.HandleFunc("/b", (&handlerB{repo: items.New()}).list)
	mux.HandleFunc("/c", (&handlerC{repo: items.New()}).list)
	_ = http.ListenAndServe(":8080", mux)
}
