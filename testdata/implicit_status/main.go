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

type APIError struct {
	Message string `json:"message"`
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /items/{id}", getItem)       // implicit 200: encode only
	mux.HandleFunc("POST /items", createItem)        // explicit 201 before the body
	mux.HandleFunc("GET /items", listItems)          // branch: explicit 400, implicit 200
	mux.HandleFunc("GET /items/{id}/raw", rawItem)   // implicit 200 through w.Write
	mux.HandleFunc("DELETE /items/{id}", dropItem)   // explicit 204, no body
	mux.HandleFunc("GET /items/{id}/etag", etagItem) // status written but unreadable

	http.ListenAndServe(":8080", mux)
}

// getItem writes a body without ever calling WriteHeader: net/http sends 200
// with the first Write.
func getItem(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// createItem states its status, so nothing is implied.
func createItem(w http.ResponseWriter, r *http.Request) {
	var in CreateItem
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(Item{Name: in.Name})
}

// listItems states the status on one branch and not on the other, so both the
// explicit 400 and the implicit 200 are real responses.
func listItems(w http.ResponseWriter, r *http.Request) {
	if r.URL.Query().Get("q") == "" {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(APIError{Message: "q is required"})
		return
	}
	_ = json.NewEncoder(w).Encode([]Item{{ID: "1"}})
}

// rawItem writes pre-marshalled bytes: still an implicit 200.
func rawItem(w http.ResponseWriter, r *http.Request) {
	b, _ := json.Marshal(Item{ID: r.PathValue("id")})
	_, _ = w.Write(b)
}

// etagItem states a status apispec cannot read (a map lookup), then writes a
// body. The status IS stated, so the implicit 200 must not be claimed here —
// this response is genuinely undetermined.
func etagItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(statusOverride[r.URL.Path])
	_ = json.NewEncoder(w).Encode(Item{ID: r.PathValue("id")})
}

// statusOverride stands in for any runtime-computed status.
var statusOverride = map[string]int{}

// dropItem states a bodyless status.
func dropItem(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNoContent)
}
