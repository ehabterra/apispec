// Fixture: the write-side status mapper pattern (issue #187). The status handed
// to http.Error is a struct FIELD (api.Status) whose value is set across the
// return branches of a mapper function (MapError) — one level deep through
// per-status helpers (mapAs404/mapAs400) that return (APIError, bool) positional
// composites, plus a direct APIError{500,...} literal. The operation must resolve
// to {400, 404, 500}, not a bare default.
package main

import (
	"errors"
	"net/http"
)

var (
	ErrNotFound   = errors.New("not found")
	ErrBadRequest = errors.New("bad request")
)

type APIError struct {
	Status  int
	Message string
}

func mapAs404(err error) (APIError, bool) {
	if errors.Is(err, ErrNotFound) {
		return APIError{http.StatusNotFound, "not found"}, true
	}
	return APIError{}, false
}

func mapAs400(err error) (APIError, bool) {
	if errors.Is(err, ErrBadRequest) {
		return APIError{http.StatusBadRequest, "bad request"}, true
	}
	return APIError{}, false
}

// MapError's returns set the Status field across branches: two come from helper
// calls (var assigned from mapAsXXX), one is a direct literal.
func MapError(err error) APIError {
	if api, ok := mapAs404(err); ok {
		return api
	}
	if api, ok := mapAs400(err); ok {
		return api
	}
	return APIError{http.StatusInternalServerError, "internal error"}
}

func writeError(w http.ResponseWriter, err error) {
	api := MapError(err)
	http.Error(w, api.Message, api.Status)
}

func getThing(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("id") == "" {
		writeError(w, ErrBadRequest)
		return
	}
	writeError(w, ErrNotFound)
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /thing", getThing)
	_ = http.ListenAndServe(":8080", mux)
}
