// Package main exercises mount composition ACROSS framework boundaries
// (issue #138): a chi router mounted under a net/http ServeMux must contribute
// its mount prefix to the routes registered on it, so /users is documented at
// /api/users rather than at the bare path the sub-router knows.
//
// The same call — mux.Handle — registers both mounts and ordinary handlers, so
// the fixture carries both shapes: only the router argument tells them apart.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Status struct {
	State string `json:"state"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]User{})
}

func getUser(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(User{})
}

// statusHandler is an ordinary http.Handler VALUE, not a router: registering it
// with mux.Handle must stay a route and must not be mistaken for a mount.
type statusHandler struct{}

// ServeHTTP reports the service status.
func (h *statusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Status{State: "ok"})
}

func main() {
	api := chi.NewRouter()
	api.Get("/users", listUsers)
	api.Get("/users/{id}", getUser)

	root := http.NewServeMux()
	// Mount: the chi router reached through http.StripPrefix, whose own result
	// type (net/http.Handler) says nothing — the router is the inner argument.
	root.Handle("/api/", http.StripPrefix("/api", api))
	// Route: a plain handler value on the same call. Held in a variable and
	// given an explicit verb, so the handler body resolves (#204) and the
	// method is not the bare ExtractRoute default.
	status := &statusHandler{}
	root.Handle("GET /status", status)

	_ = http.ListenAndServe(":8080", root)
}
