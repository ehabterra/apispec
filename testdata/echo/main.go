package main

import (
	"github.com/labstack/echo/v4"
)

// main is the entry point for the Echo example API server.
func main() {
	e := echo.New()

	v1 := e.Group("/v1")

	users := v1.Group("/users", func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			return next(c)
		}
	})

	// Register all API routes
	RegisterUsersRoutes(users, NewHandler(&defaultUserService{}))

	// Health check
	e.GET("/health", healthCheck)

	// API info
	e.GET("/api/info", getAPIInfo)

	e.Logger.Fatal(e.Start(":8080"))
}
