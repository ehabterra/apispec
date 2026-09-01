package api

import (
	"encoding/json"
	"net/http"
)

// Item is served from a package other than the one registering the route, so
// the handler identity arrives qualified with the SHORT package name while the
// route's package is the import path — the two do not match, and the method
// lookup has to resolve the receiver rather than string-compare.
type Item struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// API is a cross-package handler holder.
type API struct{}

// Items lists or deletes, dispatching on the request method.
func (a *API) Items(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode([]Item{})
	case http.MethodDelete:
		w.WriteHeader(http.StatusNoContent)
	}
}
