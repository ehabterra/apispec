package main

import (
	"net/http"

	"github.com/ehabterra/swagen/testdata/chi/payment"
	"github.com/ehabterra/swagen/testdata/chi/products"
	"github.com/ehabterra/swagen/testdata/chi/users"
	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewMux()

	// Create the user service, which contains its own router.
	userService := users.NewService()

	// Mount the user routes, injecting the user service.
	r.Mount(users.UserRoute, users.Routes(userService))
	r.Mount("/payment", payment.PaymentAPIs(nil))

	m := NewModule(WithRouter(products.Routes()))

	m.Init(r)

	http.ListenAndServe(":3000", r)
}

type Module struct {
	router chi.Router
}

func (m *Module) Init(r *chi.Mux) {
	r.Mount("/products", m.router)
}

func NewModule(opts ...func(*Module)) *Module {
	module := &Module{}

	for _, opt := range opts {
		opt(module)
	}

	return module
}

func WithRouter(router chi.Router) func(*Module) {
	return func(m *Module) {
		m.router = router
	}
}
