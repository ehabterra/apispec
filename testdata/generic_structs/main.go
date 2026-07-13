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

// Pair is a two-parameter generic — each declared parameter must map to its
// own concrete argument (K→User, V→Product), not collapse together.
type Pair[K any, V any] struct {
	First  K `json:"first"`
	Second V `json:"second"`
}

type User struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
	Avatar []byte `json:"avatar,omitempty"`
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

// getPair returns a two-parameter generic Pair[User, Product]: First→User,
// Second→Product. Guards multi-argument type-parameter substitution.
func getPair(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Pair[User, Product]{First: users[0], Second: products[0]})
}

// getNested returns a NESTED generic — Envelope[Page[User]] — where the type
// argument is itself a generic instantiation. data must resolve to the Page
// envelope (items → User), not a placeholder.
func getNested(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(Envelope[Page[User]]{Data: Page[User]{Items: users}})
}

// NewEnvelope is a generic constructor; T is INFERRED from the argument type.
func NewEnvelope[T any](data T) Envelope[T] {
	return Envelope[T]{Data: data}
}

// getInferred returns an INFERRED instantiation: NewEnvelope(products[0]) is
// Envelope[Product] with no explicit [Product] at the encode site — the type
// argument is inferred from the call. data must resolve to Product.
func getInferred(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode(NewEnvelope(products[0]))
}

// getBatch returns a SLICE of a generic instantiation — []Envelope[User] —
// exercising the wrapped form: the concrete argument must survive the slice
// constructor so the element resolves to Envelope[User], not the declaration
// placeholder (Envelope[T any]).
func getBatch(w http.ResponseWriter, r *http.Request) {
	json.NewEncoder(w).Encode([]Envelope[User]{{Data: users[0], Message: "ok"}})
}

// createPage decodes a generic REQUEST body Page[User]; it must key to the same
// clean component as the Page[User] response body (no duplicate schema).
func createPage(w http.ResponseWriter, r *http.Request) {
	var p Page[User]
	_ = json.NewDecoder(r.Body).Decode(&p)
	w.WriteHeader(http.StatusCreated)
}

var users = []User{{ID: 1, Name: "Alice", Email: "alice@example.com"}}
var products = []Product{{SKU: "A-1", Price: 9.99}}

func main() {
	http.HandleFunc("/users", listUsers)
	http.HandleFunc("/products", listProducts)
	http.HandleFunc("/user", getUser)
	http.HandleFunc("/pair", getPair)
	http.HandleFunc("/nested", getNested)
	http.HandleFunc("/inferred", getInferred)
	http.HandleFunc("/create", createPage)
	http.HandleFunc("/batch", getBatch)
	http.ListenAndServe(":8080", nil)
}
