package api

import "github.com/labstack/echo/v4"

// Handlers is the handler interface. Routes are wired through it, so resolving
// a route handler needs interface resolution — and the implementation lives in
// a *different* package (handlers), mirroring the clean-architecture layout.
type Handlers interface {
	Create() echo.HandlerFunc
	Get() echo.HandlerFunc
	Login() echo.HandlerFunc
}

// RegisterRoutes registers the routes on a group, passing each handler as a
// call returning echo.HandlerFunc (the handler-factory pattern).
func RegisterRoutes(g *echo.Group, h Handlers) {
	g.POST("/users", h.Create())
	g.GET("/users/:id", h.Get())
	g.POST("/login", h.Login())
}
