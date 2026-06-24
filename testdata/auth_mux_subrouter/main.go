// Package main demonstrates gorilla/mux subrouter middleware: r.Use on a
// subrouter guards the routes registered on it.
package main

import (
	"net/http"

	"github.com/golang-jwt/jwt/v5"
	"github.com/gorilla/mux"
)

// jwtMiddleware validates a JWT in the handler it returns.
func jwtMiddleware(next http.Handler) http.Handler {
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
	r := mux.NewRouter()
	api := r.PathPrefix("/api").Subrouter()
	api.Use(jwtMiddleware)          // guards the subrouter
	api.HandleFunc("/me", getUser)  // protected
	r.HandleFunc("/health", health) // open
	_ = http.ListenAndServe(":8080", r)
}
