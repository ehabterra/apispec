package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Widget is returned by the api package.
type Widget struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Register mounts the api routes.
func Register(r chi.Router) {
	r.Get("/widgets/{id}", getWidget)
	r.Post("/widgets", createWidget)
}

func getWidget(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Widget{ID: chi.URLParam(r, "id")})
}

func createWidget(w http.ResponseWriter, r *http.Request) {
	var in Widget
	_ = json.NewDecoder(r.Body).Decode(&in)
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(in)
}
