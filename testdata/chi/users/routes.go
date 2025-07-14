package users

import (
	"github.com/go-chi/chi/v5"
)

const UserRoute = "/users"

// Routes defines the user routes. It now accepts a Service dependency.
func Routes(s *Service) chi.Router {
	// We are now using the router from the injected service.
	s.Router.Get("/", ListUsers)
	s.Router.Get("/:id", GetUser)
	s.Router.Post("/", CreateUser)
	return s.Router
}
