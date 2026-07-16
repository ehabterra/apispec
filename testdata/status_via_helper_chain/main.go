// Fixture: status codes threaded through a chain of response helpers. A handler
// calls respondError(w, http.StatusX, ...) which forwards to
// respondJSON(w, status, ...) which finally calls w.WriteHeader(status). The
// status is a parameter across TWO hops, so a single-hop parent lookup leaves
// every error at "default"; multi-hop parameter resolution recovers the real
// codes. A shared error mapper (writeError) contributes its switch-branch
// statuses to each caller (conservative but statically reachable).
package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

type errorBody struct {
	Message string `json:"message"`
}

type Widget struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// respondJSON is the base writer — the only place WriteHeader is called.
func respondJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

// respondError forwards a status through to respondJSON (the second hop).
func respondError(w http.ResponseWriter, status int, message string) {
	respondJSON(w, status, errorBody{Message: message})
}

var (
	errNotFound  = errors.New("not found")
	errForbidden = errors.New("forbidden")
)

// writeError is a shared error→status mapper; each caller inherits every branch.
func writeError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errNotFound):
		respondError(w, http.StatusNotFound, "not found")
	case errors.Is(err, errForbidden):
		respondError(w, http.StatusForbidden, "forbidden")
	default:
		respondError(w, http.StatusInternalServerError, "internal error")
	}
}

func getWidget(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Token") == "" {
		respondError(w, http.StatusUnauthorized, "no token") // 401 via 2 hops
		return
	}
	if r.URL.Query().Get("id") == "" {
		respondError(w, http.StatusBadRequest, "missing id") // 400 via 2 hops
		return
	}
	widget, err := load(r)
	if err != nil {
		writeError(w, err) // 404 / 403 / 500 via 3 hops through the mapper
		return
	}
	respondJSON(w, http.StatusOK, widget) // 200 via 1 hop
}

func load(r *http.Request) (Widget, error) {
	return Widget{}, nil
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /widget", getWidget)
	_ = http.ListenAndServe(":8080", mux)
}
