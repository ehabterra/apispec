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

const (
	X = "x"
	Y = "y"
	Z = "z"
)

// User represents a user with validation constraints
type User struct {
	ID            int    `json:"id" validate:"required,min=1"`
	Name          string `json:"name" validate:"required,min=2,max=50"`
	Email         string `json:"email" validate:"required,email"`
	Age           int    `json:"age" validate:"min=18,max=120"`
	Status        Status `json:"status"`
	MaritalStatus string `json:"marital_status" validate:"required,oneof=single married divorced"`
	Bio           string `json:"bio" min:"10" max:"500"`
	Website       string `json:"website" pattern:"^https?://.*"`
	Country       string `json:"country" enum:"US,CA,UK,DE,FR"`
}

// GetUser returns a user
func GetUser() User {
	return User{
		ID:      1,
		Name:    "John Doe",
		Email:   "john@example.com",
		Age:     30,
		Status:  StatusActive,
		Bio:     "Software developer",
		Website: "https://johndoe.com",
		Country: "US",
	}
}

// Product represents a product with validation constraints
type Product struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func GetProduct() Product {
	return Product{
		ID:   1,
		Name: "Widget",
	}
}

// Handler handles HTTP requests
type Handler struct{}

// GetUserHandler handles GET /user requests
func (h *Handler) GetUserHandler(w http.ResponseWriter, r *http.Request) {
	user := GetUser()
	// This will trigger schema generation for User type
	w.Header().Set("Content-Type", "application/json")
	// Use json.Marshal to trigger response pattern matching
	json.Marshal(user)
}

func (h *Handler) GetProductHandler(w http.ResponseWriter, r *http.Request) {
	product := GetProduct()
	w.Header().Set("Content-Type", "application/json")
	json.Marshal(product)
}

func main() {
	handler := &Handler{}
	http.HandleFunc("/user", handler.GetUserHandler)
	http.HandleFunc("/product", handler.GetProductHandler)
	http.ListenAndServe(":8080", nil)
}
