package user

import (
	"net/http"
	"strconv"
	"time"

	"another-chi-router/internal/utils"
	"another-chi-router/models"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/render"
	"github.com/google/uuid"
)

// Handler handles user routes
type Handler struct {
	// In a real application, you would inject dependencies like:
	// userService user.Service
}

// New creates a new user handler
func New() *Handler {
	return &Handler{}
}

// Routes returns the user routes
func (h *Handler) Routes() http.Handler {
	r := chi.NewRouter()

	// User CRUD operations
	r.Get("/", h.list)          // GET /user/
	r.Get("/{name}", h.show)    // GET /user/{name}
	r.Post("/create", h.create) // POST /user/create
	r.Put("/{id}", h.update)    // PUT /user/{id}
	r.Delete("/{id}", h.delete) // DELETE /user/{id}

	// Additional user operations
	r.Get("/{id}/profile", h.getProfile)    // GET /user/{id}/profile
	r.Put("/{id}/profile", h.updateProfile) // PUT /user/{id}/profile
	r.Get("/search", h.search)              // GET /user/search

	return r
}

// list returns a list of users with pagination
func (h *Handler) list(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")

	page := 1
	limit := 20

	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = p
		}
	}

	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 100 {
			limit = l
		}
	}

	// In a real application, you would fetch users from a database
	users := []models.User{
		{
			ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			Name:      "John Doe",
			Email:     "john@example.com",
			Age:       30,
			Status:    models.UserStatusActive,
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
		{
			ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174001"),
			Name:      "Jane Smith",
			Email:     "jane@example.com",
			Age:       25,
			Status:    models.UserStatusActive,
			CreatedAt: time.Now().Add(-48 * time.Hour),
			UpdatedAt: time.Now().Add(-2 * time.Hour),
		},
	}

	response := models.UserListResponse{
		Users: users,
		Pagination: models.Pagination{
			Page:       page,
			Limit:      limit,
			Total:      len(users),
			TotalPages: 1,
		},
	}

	render.JSON(w, r, response)
}

// show returns a specific user by name
func (h *Handler) show(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Name parameter is required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would fetch the user by name from a database
	user := models.User{
		ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
		Name:      name,
		Email:     "john@example.com",
		Age:       30,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	render.JSON(w, r, user)
}

// create creates a new user
func (h *Handler) create(w http.ResponseWriter, r *http.Request) {
	var req models.CreateUserRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Invalid request body",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would create the user in a database
	user := models.User{
		ID:        uuid.New(),
		Name:      req.Name,
		Email:     req.Email,
		Age:       req.Age,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	render.Status(r, http.StatusCreated)
	render.JSON(w, r, user)
}

// update updates an existing user
func (h *Handler) update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "ID parameter is required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	var req models.UpdateUserRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		utils.ErrorResponse(w, r, models.UserError("Invalid request body"))
		return
	}

	// In a real application, you would update the user in a database
	user := models.User{
		ID:        uuid.MustParse(idStr),
		Name:      "Updated Name",
		Email:     "updated@example.com",
		Age:       35,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now(),
	}

	render.JSON(w, r, user)
}

// delete deletes a user
func (h *Handler) delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "ID parameter is required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would delete the user from a database
	render.Status(r, http.StatusNoContent)
}

// getProfile returns a user's profile
func (h *Handler) getProfile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "ID parameter is required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would fetch the user profile from a database
	user := models.User{
		ID:        uuid.MustParse(idStr),
		Name:      "John Doe",
		Email:     "john@example.com",
		Age:       30,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now().Add(-1 * time.Hour),
	}

	render.JSON(w, r, user)
}

// updateProfile updates a user's profile
func (h *Handler) updateProfile(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	if idStr == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "ID parameter is required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	var req models.UpdateUserRequest
	if err := render.DecodeJSON(r.Body, &req); err != nil {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Invalid request body",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would update the user profile in a database
	user := models.User{
		ID:        uuid.MustParse(idStr),
		Name:      "Updated Profile Name",
		Email:     "updated.profile@example.com",
		Age:       32,
		Status:    models.UserStatusActive,
		CreatedAt: time.Now().Add(-24 * time.Hour),
		UpdatedAt: time.Now(),
	}

	render.JSON(w, r, user)
}

// search searches for users
func (h *Handler) search(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	if query == "" {
		render.Status(r, http.StatusBadRequest)
		render.JSON(w, r, models.ErrorResponse{
			Error: "Query parameter 'q' is required",
			Code:  http.StatusBadRequest,
		})
		return
	}

	// In a real application, you would search users in a database
	users := []models.User{
		{
			ID:        uuid.MustParse("123e4567-e89b-12d3-a456-426614174000"),
			Name:      "John Doe",
			Email:     "john@example.com",
			Age:       30,
			Status:    models.UserStatusActive,
			CreatedAt: time.Now().Add(-24 * time.Hour),
			UpdatedAt: time.Now().Add(-1 * time.Hour),
		},
	}

	response := models.UserListResponse{
		Users: users,
		Pagination: models.Pagination{
			Page:       1,
			Limit:      20,
			Total:      len(users),
			TotalPages: 1,
		},
	}

	render.JSON(w, r, response)
}
