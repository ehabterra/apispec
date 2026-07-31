// Package common holds the house responder every handler funnels through.
package common

import (
	"encoding/json"
	"net/http"
)

func RespondWithSuccess(w http.ResponseWriter, msg string, data any, code int) {
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}

func RespondWithError(w http.ResponseWriter, code int) {
	w.WriteHeader(code)
}
