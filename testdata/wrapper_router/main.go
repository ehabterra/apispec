// Fixture: a project that wraps a router in its own type (gitea's
// modules/web.Router around *chi.Mux) and registers everything through it.
//
// Three shapes that were each broken, all in one project because a real one has
// them together:
//
//  1. chi's own patterns match the DELEGATE call inside the wrapper, where the
//     path is a call and the handler is a `[]any` — so the default run documented
//     one junk route (`/{path}` POST, `operationId: net/http.HandlerFunc`) and
//     none of the real ones.
//  2. The verb arrives as an ARGUMENT (`Methods("GET,POST", …)`), which no
//     extraction hint could read.
//  3. A handler behind a type conversion (`http.HandlerFunc(fn)`) was taken as
//     the handler itself, losing the body.
//
// registerCRUD is here as the shape that must NOT break: a helper that takes the
// router and a base path, where the inner call IS the only registration.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

type Item struct {
	SKU   string `json:"sku"`
	Count int    `json:"count"`
}

func listUsers(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]User{})
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var in User
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(in)
}

func getItem(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{})
}

// searchItems answers both verbs of one registration.
func searchItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode([]Item{})
}

// countItems is registered through a type conversion.
func countItems(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{})
}

// deleteUser is registered by a helper that receives the chi router directly.
func deleteUser(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}

// registerCRUD registers on the chi router it is handed. The path is built from a
// parameter, so this inner call is the only registration there is — nothing
// outside it states the path.
func registerCRUD(m *chi.Mux, base string) {
	m.Delete(base+"/{id}", deleteUser)
}

func main() {
	r := NewRouter()

	r.Get("/users", listUsers)
	r.Post("/users", createUser)

	// The prefix lives in a field, applied by getPattern — no sub-router exists.
	r.Group("/api", func() {
		r.Get("/items", getItem)
		// One registration, two verbs.
		r.Methods("GET,POST", "/items/search", searchItems)
		// The handler is behind a conversion.
		r.Get("/items/count", http.HandlerFunc(countItems))
	})

	registerCRUD(r.chiRouter, "/users")

	_ = http.ListenAndServe(":8080", r.chiRouter)
}
