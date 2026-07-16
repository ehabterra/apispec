// Fixture: a chi router whose audit middleware reassigns the request variable
// `r` (the canonical `r = r.WithContext(ctx)` idiom), shadowing the same name
// used for the `r chi.Router` receiver at the registration site. This pins the
// lazy-tracker regression where that cross-scope name collision made the
// router's own receiver registrations — a direct r.Get, and a whole nested
// r.Group subtree — get claimed onto the middleware producer and dropped from
// the spec, while argument-passed mount helpers (mountUsers(r)) survived.
//
// The middleware is installed via r.Use(auditGuard(reporter())) — a CALL whose
// returned closure carries the reassignment — because that is what records the
// callee-body `r` in the caller-scope call edge's AssignmentMap and triggers
// the mis-claim. Every route below must appear in the generated spec.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Tenant struct {
	ID string `json:"id"`
}

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type Cap struct {
	Name string `json:"name"`
}

// auditGuard returns a middleware whose handler reassigns `r`.
func auditGuard(report func(method, path string)) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r = r.WithContext(r.Context())
			next.ServeHTTP(w, r)
		})
	}
}

func reporter() func(method, path string) {
	return func(method, path string) {}
}

func plainMW(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}

func tenantHandler(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Tenant{})
}

type userHandler struct{}

func (h *userHandler) list(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]User{})
}

func (h *userHandler) create(w http.ResponseWriter, r *http.Request) {
	var u User
	_ = json.NewDecoder(r.Body).Decode(&u)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(u)
}

type capHandler struct{}

func (h *capHandler) caps(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Cap{})
}

// mountUsers registers on a router passed as an argument (this style always
// survived); it is here so the fixture keeps both wiring styles side by side.
func mountUsers(r chi.Router) {
	h := &userHandler{}
	r.Route("/users", func(r chi.Router) {
		r.Get("/", h.list)
		r.Post("/", h.create)
	})
}

func mountCaps(r chi.Router) {
	h := &capHandler{}
	r.Get("/caps", h.caps)
}

func mountAuth(r chi.Router) {
	h := &capHandler{}
	r.Route("/auth", func(r chi.Router) {
		r.Post("/login", h.caps)
		r.Group(func(r chi.Router) {
			r.Use(auditGuard(reporter()))
			r.Get("/me", h.caps)
		})
	})
}

func mountWorkflow(r chi.Router) {
	h := &capHandler{}
	r.Get("/workflows", h.caps)
}

func mountNotify(r chi.Router) {
	h := &capHandler{}
	r.Get("/notifications", h.caps)
}

func main() {
	r := chi.NewRouter()
	r.Route("/api/v1", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(plainMW)
			r.Use(auditGuard(reporter()))

			// Direct receiver registration in the closure that installs the
			// r-reassigning middleware — this regressed.
			r.Get("/tenant", tenantHandler)

			mountAuth(r)
			mountWorkflow(r)
			mountNotify(r)
			mountCaps(r)

			// Nested group whose whole subtree also regressed.
			r.Group(func(r chi.Router) {
				r.Use(plainMW)
				mountUsers(r)
			})
		})
	})
	_ = http.ListenAndServe(":8080", r)
}
