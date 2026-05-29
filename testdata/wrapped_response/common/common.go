// Package common is a minimal stand-in for a typical envelope-response
// helper (e.g. lmd-core's pkg/common). The envelope's payload field is
// declared as `interface{}`, so the *concrete* payload type per route
// is hidden behind the generic field unless something tells APISpec
// which constructor-argument feeds it.
package common

import (
	"encoding/json"
	"net/http"
)

// Envelope wraps every response in {message, data, code}. The Data
// field is interface{} at the type-checker level — the concrete type
// is only knowable at the call site of RespondWithSuccess.
type Envelope struct {
	Message string      `json:"message,omitempty"`
	Data    interface{} `json:"data,omitempty"`
	Code    int         `json:"code,omitempty"`
}

// NewEnvelope is the wrapper constructor whose `data` parameter binds
// directly to Envelope.Data. APISpec's field-provenance pass detects
// this binding so the schema for a given route can specialise Data
// with the caller-site payload type instead of leaving it as object.
func NewEnvelope(message string, data interface{}, code int) *Envelope {
	return &Envelope{
		Message: message,
		Data:    data,
		Code:    code,
	}
}

// RespondWithSuccess is the helper handlers call to write a successful
// JSON response. The `data` parameter flows through NewEnvelope into
// Envelope.Data; per-route specialisation walks that chain.
func RespondWithSuccess(w http.ResponseWriter, message string, data interface{}, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	response := NewEnvelope(message, data, code)
	_ = json.NewEncoder(w).Encode(response)
}
