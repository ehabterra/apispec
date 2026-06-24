// Package main demonstrates echo group middleware: a JWT applied to a group via
// echojwt protects every route in that group.
package main

import (
	echojwt "github.com/labstack/echo-jwt/v4"
	"github.com/labstack/echo/v4"
)

func me(c echo.Context) error     { return c.JSON(200, map[string]string{}) }
func health(c echo.Context) error { return c.String(200, "ok") }

func main() {
	e := echo.New()
	api := e.Group("/api", echojwt.JWT([]byte("secret"))) // group-wide JWT
	api.GET("/me", me)                                    // protected
	e.GET("/health", health)                              // open
	_ = e.Start(":8080")
}
