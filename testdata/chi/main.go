package main

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/chi/payment"
	"github.com/ehabterra/apispec/testdata/chi/products"
	"github.com/ehabterra/apispec/testdata/chi/users"
	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewMux()

	// Mount user, product, and payment services using a consistent pattern.
	r.Mount("/users", users.Routes())
	r.Mount("/products", products.Routes())
	r.Mount("/payment", payment.Routes())

	// Start the HTTP server
	http.ListenAndServe(":3000", r)
}
