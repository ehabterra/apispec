package users

import (
	"github.com/go-chi/chi/v5"
)

const UserRoute = "/users"

// Routes returns a chi.Router with all user endpoints registered.
func Routes() chi.Router {
	r := chi.NewRouter()
	r.Get("/", ListUsers)
	r.Get("/{id}", GetUser)
	r.Post("/", CreateUser)
	return r
}
