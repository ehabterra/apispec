// Package web holds the route registration, in a different package from both
// main and the command type — the arrangement every real CLI-wired project has.
package web

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// User is the public API resource.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Report is the admin resource.
type Report struct {
	Rows int `json:"rows"`
}

// RunWeb is referenced as a CROSS-PACKAGE function value (web.RunWeb), so the
// Action field holds a selector rather than a plain ident.
func RunWeb() error {
	r := chi.NewRouter()
	r.Get("/users", listUsers)
	r.Post("/users", createUser)
	return http.ListenAndServe(":8080", r)
}

// Server exists so one command's Action can be a METHOD value (srv.RunAdmin),
// the third way a function reaches the field.
type Server struct{}

// RunAdmin registers the admin surface.
func (s *Server) RunAdmin() error {
	r := chi.NewRouter()
	r.Get("/admin/report", s.report)
	return http.ListenAndServe(":8081", r)
}

func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Report{Rows: 1})
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode([]User{})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var u User
	_ = json.NewDecoder(r.Body).Decode(&u)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(u)
}
