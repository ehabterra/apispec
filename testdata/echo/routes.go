package main

import "github.com/labstack/echo/v4"

// RegisterUsersRoutes registers all API routes on the given Echo instance.
func RegisterUsersRoutes(e *echo.Group) {
	// User routes
	e.GET("/", getUsers)
	e.GET("/:id", getUser)
	e.POST("/", createUser)
	e.PUT("/:id", updateUser)
	e.DELETE("/:id", deleteUser)

}
