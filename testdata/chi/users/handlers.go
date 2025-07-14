package users

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"
)

func ListUsers(w http.ResponseWriter, r *http.Request) {
	users := []User{
		{ID: "1", Name: "Alice"},
		{ID: "2", Name: "Bob"},
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(users)
}

func CreateUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request", http.StatusBadRequest)
		return
	}
	user := User{ID: "3", Name: req.Name}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func GetUser(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	user := User{ID: id, Name: "Alice"}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}
