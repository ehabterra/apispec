package handler

import (
	"net/http"

	"another-chi-router/handler/v1/auth"
	"another-chi-router/handler/v1/user"

	"github.com/go-chi/chi/v5"
)

type Handler struct{}

func New() *Handler {
	return &Handler{}
}

func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()
	r.Mount("/", h.v1Routes())
	r.Mount("/v1", h.v1Routes())
	return r
}

func (h *Handler) v1Routes() http.Handler {
	r := chi.NewRouter()
	r.Mount("/auth", auth.New().Routes())

	r.Group(func(rg chi.Router) {
		// r.Use(authMiddleware) // In a real app, you would add auth middleware here
		rg.Mount("/user", user.New().Routes())
	})

	return r
}
