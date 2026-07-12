// Package main exercises parametric (generic) response envelopes returned
// directly at the encode site — the Page[T] / Envelope[T] pattern common in
// real APIs. The generator must resolve each concrete instantiation
// (Page[User], Page[Product], Envelope[User]) to its own schema with the type
// argument substituted into the payload field, rather than collapsing them
// onto a single placeholder.
package main

import (
	"encoding/json"
	"net/http"
)

// Page is a generic pagination envelope whose element type is a slice of the
// type parameter.
type Page[T any] struct {
	Items   []T  `json:"items"`
	Total   int  `json:"total"`
	Page    int  `json:"page"`
	HasMore bool `json:"has_more"`
}

// Envelope is a generic single-item wrapper whose payload field is a bare type
// parameter.
type Envelope[T any] struct {
	Data    T      `json:"data"`
	Message string `json:"message"`
}

type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type Product struct {
	SKU   string  `json:"sku"`
	Price float64 `json:"price"`
}

// listUsers returns a paginated envelope of users: Page[User].
func listUsers(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Page[User]{Items: users, Total: len(users)})
}

// listProducts returns a paginated envelope of products: Page[Product].
// It shares the Page[T] generic with listUsers but must NOT collapse onto the
// same schema.
func listProducts(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Page[Product]{Items: products, Total: len(products)})
}

// getUser returns a single-item envelope: Envelope[User], exercising a bare
// type-parameter payload field (Data T) rather than a slice.
func getUser(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Envelope[User]{Data: users[0], Message: "ok"})
}

var users = []User{{ID: 1, Name: "Alice", Email: "alice@example.com"}}
var products = []Product{{SKU: "A-1", Price: 9.99}}

func main() {
	http.HandleFunc("/users", listUsers)
	http.HandleFunc("/products", listProducts)
	http.HandleFunc("/user", getUser)
	http.ListenAndServe(":8080", nil)
}
