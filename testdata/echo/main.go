package main

import (
	"github.com/labstack/echo/v4"
)

// main is the entry point for the Echo example API server.
func main() {
	e := echo.New()

	g := e.Group("/users")

	// Register all API routes
	RegisterUsersRoutes(g)

	// Health check
	e.GET("/health", healthCheck)

	// API info
	e.GET("/api/info", getAPIInfo)

	e.Logger.Fatal(e.Start(":8080"))
}
