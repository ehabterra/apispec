package users

import "github.com/go-chi/chi/v5"

// Service encapsulates the router and any dependencies.
type Service struct {
	Router *chi.Mux
}

// NewService creates a new user service with its own router.
func NewService() *Service {
	r := chi.NewMux()
	// In a real app, you would initialize dependencies here.
	return &Service{
		Router: r,
	}
}
