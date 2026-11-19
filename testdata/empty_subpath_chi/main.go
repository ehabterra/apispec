// The explicit-"/" control for issue #609. chi has no empty-path form (a
// pattern must begin with "/"), so this fixture pins the other half of the
// contract: `Route("/x", …){ r.Get("/", h) }` and a mounted sub-router's
// `Get("/", h)` really register /x/, and must keep that trailing slash while
// gin/echo/fiber's `GET("")` drops it.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Item struct {
	ID int `json:"id"`
}

func listItems(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode([]Item{}) }
func getItem(w http.ResponseWriter, _ *http.Request)   { json.NewEncoder(w).Encode(Item{}) }
func listUsers(w http.ResponseWriter, _ *http.Request) { json.NewEncoder(w).Encode([]Item{}) }

func usersRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", listUsers)
	return r
}

func main() {
	r := chi.NewRouter()
	r.Route("/items", func(r chi.Router) {
		r.Get("/", listItems)
		r.Get("/{id}", getItem)
	})
	r.Mount("/users", usersRouter())
	http.ListenAndServe(":8080", r)
}
