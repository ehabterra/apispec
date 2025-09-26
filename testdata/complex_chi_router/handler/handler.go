package handler

import (
	"net/http"

	authHandler "complex-chi-router/auth"
	userHandler "complex-chi-router/user"

	"github.com/go-chi/chi/v5"
)

// Handler is the main handler that coordinates all routes
type Handler struct {
	// In a real application, you would inject dependencies like:
	// authService auth.Service
	// userService user.Service
	// logger      logger.Logger
}

// New creates a new handler instance
func New() *Handler {
	return &Handler{}
}

// Routes returns the main router with all mounted routes
// This follows the pattern: r.Mount("/api", handler.New(...).Routes())
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// Mount authentication routes at /auth
	r.Mount("/auth", authHandler.New().Routes())

	// Create a protected group for user routes
	// This demonstrates the pattern: r.Group(func(rg chi.Router) { ... })
	r.Group(func(rg chi.Router) {
		// In a real application, you would add authentication middleware here
		rg.Use(h.authMiddleware)

		// Mount user routes at /user
		rg.Mount("/user", userHandler.New().Routes())
	})

	return r
}

// authMiddleware is a placeholder for authentication middleware
// In a real application, this would validate JWT tokens, check permissions, etc.
func (h *Handler) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In a real application, you would:
		// 1. Extract the JWT token from the Authorization header
		// 2. Validate the token
		// 3. Extract user information from the token
		// 4. Add user context to the request
		// 5. Call next.ServeHTTP(w, r) if authentication succeeds
		// 6. Return 401 Unauthorized if authentication fails

		// For demo purposes, we'll just call the next handler
		next.ServeHTTP(w, r)
	})
}
