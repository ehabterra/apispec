// Package main is the chi half of the handler-wrapper attribution fixture: the
// defect is not net/http's, so the same wiring has to resolve here too
// (issue #364, golden rule #5).
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateItem struct {
	Name string `json:"name"`
}

// withLogging is the standard func(http.Handler) http.Handler middleware.
func withLogging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.Header.Get("X-Request-Id")
		next.ServeHTTP(w, r)
	})
}

// getItem returns one item.
func getItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(Item{ID: chi.URLParam(r, "id")})
}

// createItem decodes a CreateItem body and returns 201 with an Item.
func createItem(w http.ResponseWriter, r *http.Request) {
	var in CreateItem
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{Name: in.Name})
}

func main() {
	r := chi.NewRouter()

	// Wrapped at the registration site.
	r.Method("GET", "/wrapped/items/{id}", withLogging(http.HandlerFunc(getItem)))
	r.Method("POST", "/wrapped/items", withLogging(http.HandlerFunc(createItem)))

	// Controls: the same handlers registered directly.
	r.Get("/direct/items/{id}", getItem)
	r.Post("/direct/items", createItem)

	_ = http.ListenAndServe(":8080", r)
}
