package main

import (
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
)

// Router represents an HTTP router interface
type Router interface {
	Mount(pattern string, handler http.Handler)
	Get(pattern string, handlerFn http.HandlerFunc)
	Post(pattern string, handlerFn http.HandlerFunc)
}

// AppRouter represents the main application router
type AppRouter struct {
	userRouter    chi.Router
	productRouter chi.Router
	orderRouter   chi.Router
	paymentRouter chi.Router
}

// WithUserRouter returns a functional option that sets the user router
func WithUserRouter(userRouter chi.Router) func(*AppRouter) {
	return func(r *AppRouter) {
		r.userRouter = userRouter
	}
}

// WithProductRouter returns a functional option that sets the product router
func WithProductRouter(productRouter chi.Router) func(*AppRouter) {
	return func(r *AppRouter) {
		r.productRouter = productRouter
	}
}

// WithOrderRouter returns a functional option that sets the order router
func WithOrderRouter(orderRouter chi.Router) func(*AppRouter) {
	return func(r *AppRouter) {
		r.orderRouter = orderRouter
	}
}

// WithPaymentRouter returns a functional option that sets the payment router
func WithPaymentRouter(paymentRouter chi.Router) func(*AppRouter) {
	return func(r *AppRouter) {
		r.paymentRouter = paymentRouter
	}
}

// NewAppRouter creates a new application router with the given options
func NewAppRouter(options ...func(*AppRouter)) *AppRouter {
	router := &AppRouter{}

	for _, option := range options {
		option(router)
	}

	return router
}

// Routes builds and returns the main router with all sub-routers mounted
// This method demonstrates the critical tracing case: routers passed through
// functional options (WithUserRouter, WithOrderRouter, etc.) are assigned to
// struct fields and later mounted. The tracker must link the assignment of
// each router parameter to its usage in the Mount() calls.
func (r *AppRouter) Routes() *chi.Mux {
	router := chi.NewRouter()

	// Mount sub-routers that were passed through functional options
	// The tracker should trace: option parameter -> struct field assignment -> Mount() usage
	if r.userRouter != nil {
		router.Mount("/users", r.userRouter)
	}

	if r.productRouter != nil {
		router.Mount("/products", r.productRouter)
	}

	if r.orderRouter != nil {
		router.Mount("/orders", r.orderRouter)
	}

	if r.paymentRouter != nil {
		router.Mount("/payments", r.paymentRouter)
	}

	// Add a health check endpoint
	router.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	return router
}

// CreateUserRouter creates a user router with some routes
func CreateUserRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("users"))
	})
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := chi.URLParam(r, "id")
		fmt.Println(id)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("user"))
	})
	return r
}

// CreateProductRouter creates a product router with some routes
func CreateProductRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("products"))
	})
	r.Get("/{id}", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("product"))
	})
	return r
}

// CreateOrderRouter creates an order router with some routes
func CreateOrderRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("orders"))
	})
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("order created"))
	})
	return r
}

// CreatePaymentRouter creates a payment router with some routes
func CreatePaymentRouter() chi.Router {
	r := chi.NewRouter()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("payments"))
	})
	r.Post("/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte("payment created"))
	})
	return r
}

func main() {
	// Create individual routers
	userRouter := CreateUserRouter()
	productRouter := CreateProductRouter()
	orderRouter := CreateOrderRouter()
	paymentRouter := CreatePaymentRouter()

	// Create the main app router using functional options
	// Each router is passed as a parameter and needs to be traced to its Mount() usage
	appRouter := NewAppRouter(
		WithUserRouter(userRouter),
		WithProductRouter(productRouter),
		WithOrderRouter(orderRouter),
		WithPaymentRouter(paymentRouter),
	)

	// Build the routes - this is where the routers are mounted
	router := appRouter.Routes()

	http.ListenAndServe(":8080", router)
}
