package main

import (
	"encoding/json"
	"net/http"
)

// Status represents different status values
type Status string

// Status constants
const (
	StatusActive   Status = "active"
	StatusInactive Status = "inactive"
	StatusPending  Status = "pending"
)

// Priority represents different priority levels
type Priority int

// Priority constants using iota
const (
	PriorityLow Priority = iota
	PriorityMedium
	PriorityHigh
	PriorityCritical
)

// User represents a user with validation constraints
type User struct {
	ID       int      `json:"id" validate:"required,min=1"`
	Name     string   `json:"name" validate:"required,min=2,max=50"`
	Email    string   `json:"email" validate:"required,regexp=^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\\.[a-zA-Z]{2,5}$"`
	Age      int      `json:"age" validate:"min=18,max=120"`
	Status   Status   `json:"status"`
	Priority Priority `json:"priority"`
	Bio      string   `json:"bio" min:"10" max:"500"`
	Website  string   `json:"website" validate:"regexp=^https?://.*"`
	Country  string   `json:"country" enum:"US,CA,UK,DE,FR"`
}

// UserHandler handles user-related HTTP requests
type UserHandler struct{}

// CreateUser creates a new user
func (h *UserHandler) CreateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Simulate creating user
	user.ID = 1
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(user)
}

// GetUser retrieves a user by ID
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	// Simulate getting user
	user := User{
		ID:       1,
		Name:     "John Doe",
		Email:    "john@example.com",
		Age:      30,
		Status:   StatusActive,
		Priority: PriorityMedium,
		Bio:      "Software developer with 5 years experience",
		Website:  "https://johndoe.com",
		Country:  "US",
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// UpdateUser updates an existing user
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	var user User
	if err := json.NewDecoder(r.Body).Decode(&user); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Simulate updating user
	user.ID = 1
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

// DeleteUser deletes a user
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	// Simulate deleting user
	w.WriteHeader(http.StatusNoContent)
}

type Product struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func main() {
	handler := &UserHandler{}

	// Set up HTTP routes using net/http
	http.HandleFunc("/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			handler.CreateUser(w, r)
		case http.MethodGet:
			handler.GetUser(w, r)
		}
	})

	http.HandleFunc("/users/", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handler.GetUser(w, r)
		case http.MethodPut:
			handler.UpdateUser(w, r)
		case http.MethodDelete:
			handler.DeleteUser(w, r)
		}
	})

	http.HandleFunc("/products/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			var product Product
			if err := json.NewDecoder(r.Body).Decode(&product); err != nil {
				http.Error(w, "Invalid JSON", http.StatusBadRequest)
				return
			}
			// Simulate creating product
			product.ID = 1
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(product)
		}
	})

	// Start server
	http.ListenAndServe(":8080", nil)
}
