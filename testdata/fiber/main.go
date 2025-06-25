package main

import (
	"net/http"
	"strconv"

	"github.com/gofiber/fiber/v2"
)

// User represents a user model
type User struct {
	ID    int    `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CreateUserRequest represents the request body for creating a user
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UpdateUserRequest represents the request body for updating a user
type UpdateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Code    string `json:"code"`
	Details string `json:"details,omitempty"`
}

// SuccessResponse represents a success response
type SuccessResponse struct {
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

var users = []User{
	{ID: 1, Name: "John Doe", Email: "john@example.com"},
	{ID: 2, Name: "Jane Smith", Email: "jane@example.com"},
}

func main() {
	app := fiber.New()

	// Routes
	app.Get("/users", getUsers)
	app.Get("/users/:id", getUser)
	app.Post("/users", createUser)
	app.Put("/users/:id", updateUser)
	app.Delete("/users/:id", deleteUser)
	app.Get("/health", healthCheck)
	app.Get("/api/info", getAPIInfo)

	app.Listen(":8080")
}

// Get all users
func getUsers(c *fiber.Ctx) error {
	return c.Status(http.StatusOK).JSON(users)
}

// Get a user by ID
func getUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "Invalid user ID",
		})
	}

	for _, user := range users {
		if user.ID == id {
			return c.Status(http.StatusOK).JSON(user)
		}
	}

	return c.Status(http.StatusNotFound).JSON(ErrorResponse{
		Error: "User not found",
		Code:  "USER_NOT_FOUND",
	})
}

// Create a new user
func createUser(c *fiber.Ctx) error {
	var req CreateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(map[string]interface{}{
			"error":   "Invalid request body",
			"code":    "INVALID_BODY",
			"details": err.Error(),
		})
	}

	// Validate required fields
	if req.Name == "" {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error: "Name is required",
			Code:  "MISSING_NAME",
		})
	}

	if req.Email == "" {
		return c.Status(http.StatusBadRequest).JSON(map[string]string{
			"error": "Email is required",
		})
	}

	// Generate new ID
	newUser := User{
		ID:    len(users) + 1,
		Name:  req.Name,
		Email: req.Email,
	}
	users = append(users, newUser)

	return c.Status(http.StatusCreated).JSON(SuccessResponse{
		Message: "User created successfully",
		Data:    newUser,
	})
}

// Update an existing user
func updateUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(map[string]interface{}{
			"error": "Invalid user ID",
		})
	}

	var req UpdateUserRequest
	if err := c.BodyParser(&req); err != nil {
		return c.Status(http.StatusBadRequest).JSON(ErrorResponse{
			Error:   "Invalid request body",
			Code:    "INVALID_BODY",
			Details: err.Error(),
		})
	}

	for i, existingUser := range users {
		if existingUser.ID == id {
			updatedUser := User{
				ID:    id,
				Name:  req.Name,
				Email: req.Email,
			}
			users[i] = updatedUser
			return c.Status(http.StatusOK).JSON(SuccessResponse{
				Message: "User updated successfully",
				Data:    updatedUser,
			})
		}
	}

	return c.Status(http.StatusNotFound).JSON(map[string]string{
		"error": "User not found",
	})
}

// Delete a user
func deleteUser(c *fiber.Ctx) error {
	idStr := c.Params("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.Status(http.StatusBadRequest).JSON(map[string]interface{}{
			"error": "Invalid user ID",
		})
	}

	for i, user := range users {
		if user.ID == id {
			users = append(users[:i], users[i+1:]...)
			return c.Status(http.StatusOK).JSON(map[string]string{
				"message": "User deleted successfully",
			})
		}
	}

	return c.Status(http.StatusNotFound).JSON(ErrorResponse{
		Error: "User not found",
		Code:  "USER_NOT_FOUND",
	})
}

// Health check endpoint
func healthCheck(c *fiber.Ctx) error {
	return c.Status(http.StatusOK).JSON(map[string]interface{}{
		"status": "healthy",
		"time":   "2024-01-01T00:00:00Z",
	})
}

// API info endpoint
func getAPIInfo(c *fiber.Ctx) error {
	return c.Status(http.StatusOK).JSON(map[string]interface{}{
		"name":      "Fiber API",
		"version":   "1.0.0",
		"framework": "Fiber",
	})
}
