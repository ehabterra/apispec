package main

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

// UserService defines the interface for user-related operations.
type UserService interface {
	ListUsers() []User
}

// defaultUserService is a concrete implementation of UserService.
type defaultUserService struct{}

// ListUsers returns a static list of users.
func (s *defaultUserService) ListUsers() []User {
	return []User{
		{ID: 1, Name: "John Doe", Age: 30},
		{ID: 2, Name: "Jane Smith", Age: 25},
	}
}

var userService UserService = &defaultUserService{}

// getUsers returns a list of users using the UserService interface.
func getUsers(c echo.Context) error {
	users := userService.ListUsers()
	return c.JSON(http.StatusOK, users)
}

// getUser returns a single user by ID.
func getUser(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Invalid user ID",
		})
	}

	if id <= 0 {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "User not found",
			Code:    404,
			Message: "The requested user does not exist",
		})
	}

	user := User{ID: id, Name: "John Doe", Age: 30}
	return c.JSON(http.StatusOK, user)
}

// createUser creates a new user.
func createUser(c echo.Context) error {
	var req CreateUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error":   "Invalid request body",
			"details": err.Error(),
		})
	}

	// Simulate validation
	if req.Name == "" {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Validation failed",
			Code:    400,
			Message: "Name is required",
		})
	}

	if req.Age < 0 || req.Age > 150 {
		return c.JSON(http.StatusBadRequest, map[string]string{
			"error": "Age must be between 0 and 150",
		})
	}

	user := User{ID: 123, Name: req.Name, Age: req.Age}
	return c.JSON(http.StatusCreated, SuccessResponse{
		Status:  "success",
		Message: "User created successfully",
		Data:    user,
	})
}

// updateUser updates an existing user.
func updateUser(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]interface{}{
			"error": "Invalid user ID",
		})
	}

	var req UpdateUserRequest
	if err := c.Bind(&req); err != nil {
		return c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "Invalid request body",
			Code:    400,
			Message: err.Error(),
		})
	}

	if id <= 0 {
		return c.JSON(http.StatusNotFound, map[string]string{
			"error": "User not found",
		})
	}

	user := User{ID: id, Name: req.Name, Age: req.Age}
	return c.JSON(http.StatusOK, SuccessResponse{
		Status:  "success",
		Message: "User updated successfully",
		Data:    user,
	})
}

// deleteUser deletes a user by ID.
func deleteUser(c echo.Context) error {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		return c.JSON(http.StatusBadRequest, map[string]any{
			"error": "Invalid user ID",
		})
	}

	if id <= 0 {
		return c.JSON(http.StatusNotFound, ErrorResponse{
			Error:   "User not found",
			Code:    404,
			Message: "The requested user does not exist",
		})
	}

	return c.JSON(http.StatusOK, map[string]string{
		"status":  "success",
		"message": "User deleted successfully",
	})
}

// healthCheck returns the health status of the API.
func healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": "2024-01-01T00:00:00Z",
		"version":   "1.0.0",
	})
}

// getAPIInfo returns information about the API.
func getAPIInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"name":        "Echo API",
		"version":     "1.0.0",
		"description": "A sample Echo API for testing",
		"endpoints":   []string{"/users", "/health", "/api/info"},
	})
}
