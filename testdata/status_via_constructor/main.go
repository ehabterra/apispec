package main

import (
	"encoding/json"
	"net/http"
)

// This fixture pins the "status through a constructor field" shape common
// in error-helper packages:
//
//	e := NewAPIError("missing token", http.StatusUnauthorized)
//	RespondWithError(w, e)          // does w.WriteHeader(err.Code)
//
// Resolving the 401 requires following err.Code (a selector on a parameter)
// back through the caller's argument, the assignment from the constructor
// call, and the constructor's composite-literal field Code: code — the last
// hop (return-field ↔ constructor-parameter provenance) is the missing link.

// APIError is the error body every failure path encodes.
type APIError struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// NewAPIError is the constructor carrying the status into a struct field.
func NewAPIError(message string, code int) *APIError {
	return &APIError{Message: message, Code: code}
}

// RespondWithError writes the error with its embedded status.
func RespondWithError(w http.ResponseWriter, err *APIError) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(err.Code)
	_ = json.NewEncoder(w).Encode(err)
}

// Profile is the success body.
type Profile struct {
	Email string `json:"email"`
}

func getProfile(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-Token") == "" {
		e := NewAPIError("missing token", http.StatusUnauthorized)
		RespondWithError(w, e)
		return
	}
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Profile{Email: "a@b.c"})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /profile", getProfile)
	_ = http.ListenAndServe(":8080", mux)
}
