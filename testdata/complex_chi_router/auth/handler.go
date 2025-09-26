package auth

import (
	"net/http"
	"time"

	"complex-chi-router/models"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

// Handler handles authentication routes
type Handler struct {
	// In a real application, you would inject dependencies like:
	// authService auth.Service
	// userService user.Service
}

// New creates a new auth handler
func New() *Handler {
	return &Handler{}
}

// Routes returns the authentication routes
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// Public routes (no authentication required)
	r.Post("/login", h.login)
	r.Post("/register", h.register)
	r.Post("/refresh", h.refreshToken)

	// Protected routes (authentication required)
	r.Group(func(rg chi.Router) {
		// In a real app, you would add auth middleware here
		// rg.Use(h.authMiddleware)
		rg.Post("/logout", h.logout)
		rg.Get("/me", h.getCurrentUser)
	})

	return r
}

// login handles user login
func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	var req models.LoginRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Invalid request body",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would validate credentials here
	// For demo purposes, we'll create a mock response
	user := models.User{
		ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		Name:      "John Doe",
		Email:     req.Email,
		Age:       30,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	response := models.AuthResponse{
		Token:     "mock-jwt-token",
		User:      user,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	render.JSON(w, r, response)
}

// register handles user registration
func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	var req models.RegisterRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Invalid request body",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would create the user here
	user := models.User{
		ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		Name:      req.Name,
		Email:     req.Email,
		Age:       req.Age,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	response := models.AuthResponse{
		Token:     "mock-jwt-token",
		User:      user,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, response)
}

// refreshToken handles token refresh
func (h *Handler) refreshToken(w http.ResponseWriter, r *http.Request) {
	var req models.RefreshTokenRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Invalid request body",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would validate the refresh token here
	response := models.AuthResponse{
		Token:     "new-mock-jwt-token",
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	render.JSON(w, r, response)
}

// logout handles user logout
func (h *Handler) logout(w http.ResponseWriter, r *http.Request) {
	// In a real application, you would invalidate the token here
	render.JSON(w, r, map[string]string{
		"message": "Successfully logged out",
	})
}

// getCurrentUser returns the current authenticated user
func (h *Handler) getCurrentUser(w http.ResponseWriter, r *http.Request) {
	// In a real application, you would get the user from the JWT token
	user := models.User{
		ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		Name:      "John Doe",
		Email:     "john@example.com",
		Age:       30,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	render.JSON(w, r, user)
}
