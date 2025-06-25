package main

import (
	"net/http"
	"strconv"

	"github.com/labstack/echo/v4"
)

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type CreateUserRequest struct {
	Name string `json:"name" validate:"required"`
	Age  int    `json:"age" validate:"min=0,max=150"`
}

type UpdateUserRequest struct {
	Name string `json:"name,omitempty"`
	Age  int    `json:"age,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type SuccessResponse struct {
	Status  string      `json:"status"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

func main() {
	e := echo.New()

	// User routes
	e.GET("/users", getUsers)
	e.GET("/users/:id", getUser)
	e.POST("/users", createUser)
	e.PUT("/users/:id", updateUser)
	e.DELETE("/users/:id", deleteUser)

	// Health check
	e.GET("/health", healthCheck)

	// API info
	e.GET("/api/info", getAPIInfo)

	e.Logger.Fatal(e.Start(":8080"))
}

func getUsers(c echo.Context) error {
	users := []User{
		{ID: 1, Name: "John Doe", Age: 30},
		{ID: 2, Name: "Jane Smith", Age: 25},
	}
	return c.JSON(http.StatusOK, users)
}

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

func healthCheck(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": "2024-01-01T00:00:00Z",
		"version":   "1.0.0",
	})
}

func getAPIInfo(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]any{
		"name":        "Echo API",
		"version":     "1.0.0",
		"description": "A sample Echo API for testing",
		"endpoints":   []string{"/users", "/health", "/api/info"},
	})
}
