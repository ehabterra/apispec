// Package main answers errors by RETURNING them, the echo way (issue #556).
//
// echo's HTTPErrorHandler renders a returned *echo.HTTPError: the status it
// carries, and `{"message": …}`. Nothing called on the context writes it, so
// before, every such error went undocumented.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

// Item is the success body.
type Item struct {
	ID string `json:"id"`
}

// requireToken is middleware that refuses before the handler runs.
func requireToken(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		if c.Request().Header.Get("Authorization") == "" {
			return echo.NewHTTPError(http.StatusUnauthorized, "missing token")
		}
		return next(c)
	}
}

func get(c echo.Context) error {
	if c.Param("id") == "" {
		return echo.NewHTTPError(http.StatusNotFound, "not found")
	}
	if c.Param("id") == "locked" {
		// A sentinel: a package variable, not a call — NOT yet documented.
		return echo.ErrForbidden
	}
	return c.JSON(http.StatusOK, Item{})
}

func main() {
	e := echo.New()
	e.GET("/items/:id", get, requireToken)
	_ = e.Start(":8080")
}
