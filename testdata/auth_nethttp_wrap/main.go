// Package main demonstrates net/http handler-wrapping auth: a custom middleware
// wraps the handler and validates a JWT inside the http.Handler it returns.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

// User is what the protected handler answers with. The handlers below carry a
// real body and status on purpose: with empty bodies, an operation attributed to
// the MIDDLEWARE instead of the handler looked identical to a correct one, so
// this fixture could not catch that regression (issue #364).
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Status is the open endpoint's answer.
type Status struct {
	OK bool `json:"ok"`
}

// jwtAuth is a custom middleware whose returned closure validates a JWT via
// golang-jwt. apispec looks through it to jwt.Parse and marks wrapped routes as
// bearerAuth.
func jwtAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = jwt.Parse(r.Header.Get("Authorization"), func(t *jwt.Token) (interface{}, error) {
			return nil, nil
		})
		next.ServeHTTP(w, r)
	})
}

// getUser answers with the requested user.
func getUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(User{ID: r.PathValue("id")})
}

// health answers with the service status.
func health(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Status{OK: true})
}

func main() {
	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", jwtAuth(http.HandlerFunc(getUser))) // protected
	mux.HandleFunc("GET /health", health)                             // open
	_ = http.ListenAndServe(":8080", mux)
}
