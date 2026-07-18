// Package common holds the shared error constructor and responder — in a
// DIFFERENT package from the handler that sets the branched status. This is the
// cross-package shape that previously collapsed the status to `default`: the
// call `common.NewAPIError(...)` has a selector Fun (`common.NewAPIError`), whose
// name lives in .Sel, so the resolver must read the selector field, not the
// selector itself.
package common

import (
	"encoding/json"
	"net/http"
)

type APIError struct {
	Message string `json:"message"`
	Code    int    `json:"-"`
}

func NewAPIError(message string, code int) *APIError { return &APIError{Message: message, Code: code} }

func RespondWithError(w http.ResponseWriter, err *APIError) {
	w.WriteHeader(err.Code)
	_ = json.NewEncoder(w).Encode(err)
}
