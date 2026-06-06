package main

import (
	"encoding/json"
	"net/http"
)

type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CreateUserRequest struct {
	Name string `json:"name"`
}

func main() {
	mux := http.NewServeMux()

	// Go 1.22 method-aware routing: the HTTP method is carried on the
	// registration pattern itself, alongside path wildcards.
	mux.HandleFunc("GET /users/{id}", getUser) // method + wildcard
	mux.HandleFunc("POST /users", createUser)
	mux.HandleFunc("GET /health", health)

	http.ListenAndServe(":8080", mux)
}

func getUser(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	user := User{ID: id, Name: "John"}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(user)
}

func createUser(w http.ResponseWriter, r *http.Request) {
	var req CreateUserRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	user := User{ID: "3", Name: req.Name}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(user)
}

func health(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}
