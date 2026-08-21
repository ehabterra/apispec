// Package main registers the same handlers wrapped in middleware at the
// registration site and directly, so the two operations can be compared: a
// wrapper must not replace the handler it wraps (issue #364).
package main

import (
	"encoding/json"
	"net/http"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateItem struct {
	Name string `json:"name"`
}

// withLogging is the standard func(http.Handler) http.Handler middleware shape.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get("X-Request-Id")
		next.ServeHTTP(w, r)
	})
}

// withAuth is a second wrapper, so a chain can be nested.
func withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get("Authorization")
		next.ServeHTTP(w, r)
	})
}

// withTiming is the HandlerFunc-shaped middleware: it takes and returns
// http.HandlerFunc, so the handler it wraps is a plain ident rather than a
// conversion.
func withTiming(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		next(w, r)
	}
}

// getItem returns one item.
func getItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// createItem decodes a CreateItem body and returns 201 with an Item.
func createItem(w http.ResponseWriter, r *http.Request) {
	var in CreateItem
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{Name: in.Name})
}

func main() {
	mux := http.NewServeMux()

	// Wrapped at the registration site — the operation must still be the
	// handler's, not the middleware's.
	mux.Handle("GET /wrapped/items/{id}", withLogging(http.HandlerFunc(getItem)))
	mux.Handle("POST /wrapped/items", withLogging(http.HandlerFunc(createItem)))

	// Nested wrappers: the innermost handler is still the answer.
	mux.Handle("GET /chained/items/{id}", withAuth(withLogging(http.HandlerFunc(getItem))))

	// The HandlerFunc-shaped wrapper: same requirement, plain-ident argument.
	mux.HandleFunc("GET /timed/items/{id}", withTiming(getItem))

	// Controls: the same handlers registered directly.
	mux.HandleFunc("GET /direct/items/{id}", getItem)
	mux.HandleFunc("POST /direct/items", createItem)

	_ = http.ListenAndServe(":8080", mux)
}
