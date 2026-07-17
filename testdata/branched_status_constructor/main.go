// Fixture: a status set across switch/if branches, handed to an error
// constructor, then written by a shared error helper (issue #155). Previously
// the status resolved to the branch variable — an ident, not a literal — so the
// operation got a single `default`. Fanning the variable's branch assignments
// out recovers the concrete {400, 404, 500}. Covers the inline constructor form,
// the variable form (e := NewAPIError(...)), and an if-based branch set.
package main

import (
	"encoding/json"
	"errors"
	"net/http"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrValidation = errors.New("validation")
)

type APIError struct {
	Message string `json:"message"`
	Code    int    `json:"-"`
}

func NewAPIError(msg string, code int) *APIError { return &APIError{Message: msg, Code: code} }

// RespondWithError writes the constructor's Code as the status.
func RespondWithError(w http.ResponseWriter, e *APIError) {
	w.WriteHeader(e.Code)
	_ = json.NewEncoder(w).Encode(e)
}

// writeError sets the status across switch branches, then hands it to the
// constructor inline — the shape from issue #155.
func writeError(w http.ResponseWriter, err error) {
	var statusCode int
	switch {
	case errors.Is(err, ErrNotFound):
		statusCode = http.StatusNotFound
	case errors.Is(err, ErrValidation):
		statusCode = http.StatusBadRequest
	default:
		statusCode = http.StatusInternalServerError
	}
	RespondWithError(w, NewAPIError(err.Error(), statusCode))
}

// writeErrorVar is the variable form: e := NewAPIError(...); Respond(w, e).
func writeErrorVar(w http.ResponseWriter, err error) {
	var statusCode int
	if errors.Is(err, ErrNotFound) {
		statusCode = http.StatusNotFound
	} else {
		statusCode = http.StatusInternalServerError
	}
	e := NewAPIError(err.Error(), statusCode)
	RespondWithError(w, e)
}

// getThing reports every error through writeError, so its concrete error
// statuses come only from the branch set.
func getThing(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		writeError(w, ErrValidation)
		return
	}
	writeError(w, ErrNotFound)
}

// getOther reports errors through the variable-form helper.
func getOther(w http.ResponseWriter, r *http.Request) {
	writeErrorVar(w, ErrNotFound)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /thing", getThing)
	mux.HandleFunc("GET /other", getOther)
	_ = http.ListenAndServe(":8080", mux)
}
