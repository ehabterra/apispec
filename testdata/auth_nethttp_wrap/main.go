// Package main demonstrates net/http handler-wrapping auth: a custom middleware
// wraps the handler and validates a JWT inside the http.Handler it returns.
package main

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
)

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

func getUser(w http.ResponseWriter, r *http.Request) {}
func health(w http.ResponseWriter, r *http.Request)  {}

func main() {
	mux := http.NewServeMux()
	mux.Handle("GET /users/{id}", jwtAuth(http.HandlerFunc(getUser))) // protected
	mux.HandleFunc("GET /health", health)                             // open
	_ = http.ListenAndServe(":8080", mux)
}
