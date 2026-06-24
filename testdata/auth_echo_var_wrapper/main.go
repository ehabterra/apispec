// Package main demonstrates auth detection through a middleware that is built by
// a custom constructor, stored in a local variable, and then passed to an echo
// Group. This is the golang-echo-realworld-example-app pattern:
//
//	mw := authMiddleware(secret) // constructor returning echo.MiddlewareFunc
//	api := e.Group("/api", mw)   // variable used as the group middleware
//
// Resolving it requires (1) tracing the `mw` variable back to authMiddleware
// and (2) looking through authMiddleware's body to the golang-jwt Parse call
// that identifies the scheme. The articles-style inline form (Group("/x",
// authMiddleware(secret))) is also covered for contrast.
package main

import (
	"github.com/golang-jwt/jwt/v5"
	"github.com/labstack/echo/v4"
)

// authMiddleware is a custom constructor: its returned closure validates a JWT
// via golang-jwt, so look-through must reach jwt.Parse to resolve the scheme.
func authMiddleware(secret string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			tok := c.Request().Header.Get("Authorization")
			_, err := jwt.Parse(tok, func(t *jwt.Token) (interface{}, error) {
				return []byte(secret), nil
			})
			if err != nil {
				return c.JSON(401, map[string]string{"error": "unauthorized"})
			}
			return next(c)
		}
	}
}

func me(c echo.Context) error      { return c.JSON(200, map[string]string{}) }
func profile(c echo.Context) error { return c.JSON(200, map[string]string{}) }
func health(c echo.Context) error  { return c.String(200, "ok") }

func main() {
	e := echo.New()

	mw := authMiddleware("secret") // middleware stored in a variable

	user := e.Group("/user", mw) // variable used as group middleware
	user.GET("", me)             // protected

	// Inline form: the constructor call is passed directly.
	profiles := e.Group("/profiles", authMiddleware("secret"))
	profiles.GET("/:name", profile) // protected

	e.GET("/health", health) // open
	_ = e.Start(":8080")
}
