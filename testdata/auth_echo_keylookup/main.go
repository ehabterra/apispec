// Package main configures WHERE an API key travels, which is what an apiKey
// scheme has to document (issue #370). Echo's KeyAuth defaults to
// `header:Authorization`, and every group here that says otherwise used to be
// documented at that default — so a generated client sent `Authorization:` to
// an endpoint that only reads `?api_key=`.
//
// Three groups, three answers, on purpose: two configured differently and one
// left at the default. They must not collapse onto one scheme, and the default
// one must keep the default rather than inherit a configured group's location.
package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func items(c echo.Context) error   { return c.JSON(200, map[string]string{}) }
func session(c echo.Context) error { return c.JSON(200, map[string]string{}) }
func plain(c echo.Context) error   { return c.JSON(200, map[string]string{}) }
func health(c echo.Context) error  { return c.String(200, "ok") }

func validate(key string, c echo.Context) (bool, error) { return key == "secret", nil }

func main() {
	e := echo.New()

	// The key travels as a query parameter.
	q := e.Group("/api")
	q.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup: "query:api_key",
		Validator: validate,
	}))
	q.GET("/items", items)

	// …and as a cookie here.
	c := e.Group("/session")
	c.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup: "cookie:token",
		Validator: validate,
	}))
	c.GET("/me", session)

	// Nothing configured: the library default (header:Authorization) is correct.
	d := e.Group("/default")
	d.Use(middleware.KeyAuth(validate))
	d.GET("/plain", plain)

	e.GET("/health", health) // open
	_ = e.Start(":8080")
}
