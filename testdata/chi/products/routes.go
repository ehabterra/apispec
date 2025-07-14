package products

import (
	"github.com/go-chi/chi/v5"
)

const (
	root = "/"
)

func Routes() chi.Router {
	r := chi.NewRouter()
	r.Get(root, ListProducts)
	r.Post("/", CreateProduct)
	r.Get("/:id", GetProduct)
	return r
}
