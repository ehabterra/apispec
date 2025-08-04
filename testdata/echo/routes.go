package main

import "github.com/labstack/echo/v4"

type handler interface {
	CreateUser(c echo.Context) error
	DeleteUser(c echo.Context) error
	GetUser(c echo.Context) error
	GetUsers(c echo.Context) error
	UpdateUser(c echo.Context) error
}

// RegisterUsersRoutes registers all API routes on the given Echo instance.
func RegisterUsersRoutes(e *echo.Group, h handler) {
	// User routes
	e.GET("/", h.GetUsers)
	e.GET("/:id", h.GetUser)
	e.POST("/", h.CreateUser)
	e.PUT("/:id", h.UpdateUser)
	e.DELETE("/:id", h.DeleteUser)

}
