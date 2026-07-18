package main

import (
	"errors"
	"net/http"

	"cross_package_constructor_status/common"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrValidation = errors.New("validation")
)

// paymentError sets the status across branches, then hands it to the
// cross-package error constructor + responder.
func paymentError(w http.ResponseWriter, err error) {
	var statusCode int
	switch {
	case errors.Is(err, ErrNotFound):
		statusCode = http.StatusNotFound
	case errors.Is(err, ErrValidation):
		statusCode = http.StatusBadRequest
	default:
		statusCode = http.StatusInternalServerError
	}
	common.RespondWithError(w, common.NewAPIError("error", statusCode))
}

func doReserve(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		paymentError(w, ErrValidation)
		return
	}
	paymentError(w, ErrNotFound)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /reserve", doReserve)
	_ = http.ListenAndServe(":8080", mux)
}
