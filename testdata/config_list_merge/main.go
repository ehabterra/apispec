// Package main is a fixture for issue #571: a pattern list named in --config
// used to REPLACE the built-in list, so a config adding one response pattern
// for a house helper silently dropped every built-in response pattern.
//
// apispec.yaml adds exactly one pattern (for Respond). With the lists layered,
// all three operations keep their bodies: getItem through the added pattern,
// listItems and exportPDF through the built-ins the config never mentions.
package main

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Item struct {
	ID string `json:"id"`
}

var pdf = []byte("%PDF-1.7")

// Respond is a house helper the config documents by its own pattern.
func Respond(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func getItem(w http.ResponseWriter, r *http.Request) {
	Respond(w, http.StatusOK, Item{ID: chi.URLParam(r, "id")})
}

func listItems(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode([]Item{})
}

func exportPDF(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/pdf")
	_, _ = w.Write(pdf)
}

func main() {
	r := chi.NewRouter()
	r.Get("/items", listItems)
	r.Get("/items/{id}", getItem)
	r.Get("/export", exportPDF)
	_ = http.ListenAndServe(":8080", r)
}
