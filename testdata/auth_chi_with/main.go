// Package main demonstrates chi's r.With(mw).Get(...) inline middleware: only
// the chained route is guarded, not sibling routes on the same router.
package main

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/golang-jwt/jwt/v5"
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
	r := chi.NewRouter()
	r.With(jwtMiddleware).Get("/users/{id}", getUser) // protected (chained)
	r.Get("/health", health)                          // open (sibling)
	_ = http.ListenAndServe(":8080", r)
}
