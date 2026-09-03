// Package main is about a credential documented twice (issue #412): the header
// read that a security scheme was derived from was ALSO emitted as an ordinary
// header parameter, so a generated client got a second, manually-supplied
// argument beside the one the scheme drives.
//
// Suppressing it cannot be done by name. The routes here are the four answers
// a fix has to give, and three of them keep their parameter:
//
//   - /basic/items   — an http scheme consumes Authorization: dropped;
//   - /header/items  — an apiKey scheme consumes X-API-Key: dropped, while the
//     unrelated X-Request-Id the handler also reads survives;
//   - /query/items   — the apiKey travels as a QUERY parameter, so the
//     Authorization the handler reads is a different credential: kept;
//   - /open          — no security at all, so the header is the handler's own
//     business: kept.
package main

import (
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

// open reads Authorization for its own purposes, on an unprotected route.
func open(c echo.Context) error {
	_ = c.Request().Header.Get("Authorization")
	return c.JSON(200, map[string]string{})
}

// queryKey is guarded by a key in a query parameter and reads Authorization
// itself — a different credential from the one the scheme governs.
func queryKey(c echo.Context) error {
	_ = c.Request().Header.Get("Authorization")
	return c.JSON(200, map[string]string{})
}

// headerKey is guarded by a key in X-API-Key, and also reads an unrelated
// header of its own.
func headerKey(c echo.Context) error {
	_ = c.Request().Header.Get("X-API-Key")
	_ = c.Request().Header.Get("X-Request-Id")
	return c.JSON(200, map[string]string{})
}

// basicAuthed is guarded by HTTP basic auth, which carries its credential in
// Authorization — the header this handler reads.
func basicAuthed(c echo.Context) error {
	_ = c.Request().Header.Get("Authorization")
	return c.JSON(200, map[string]string{})
}

func validateKey(key string, c echo.Context) (bool, error) { return key == "secret", nil }

func validateBasic(user, pass string, c echo.Context) (bool, error) { return user == "u", nil }

func main() {
	e := echo.New()

	e.GET("/open", open)

	q := e.Group("/query")
	q.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup: "query:api_key",
		Validator: validateKey,
	}))
	q.GET("/items", queryKey)

	h := e.Group("/header")
	h.Use(middleware.KeyAuthWithConfig(middleware.KeyAuthConfig{
		KeyLookup: "header:X-API-Key",
		Validator: validateKey,
	}))
	h.GET("/items", headerKey)

	b := e.Group("/basic")
	b.Use(middleware.BasicAuth(validateBasic))
	b.GET("/items", basicAuthed)

	_ = e.Start(":8080")
}
