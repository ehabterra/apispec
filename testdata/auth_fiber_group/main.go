// Package main demonstrates fiber group middleware: a JWT applied to a group via
// gofiber/contrib/jwt protects routes registered on the group.
package main

import (
	jwtware "github.com/gofiber/contrib/jwt"
	"github.com/gofiber/fiber/v2"
)

func me(c *fiber.Ctx) error     { return c.JSON(fiber.Map{}) }
func health(c *fiber.Ctx) error { return c.SendString("ok") }

func main() {
	app := fiber.New()
	api := app.Group("/api")
	api.Use(jwtware.New(jwtware.Config{SigningKey: jwtware.SigningKey{Key: []byte("secret")}}))
	api.Get("/me", me)         // protected
	app.Get("/health", health) // open
	_ = app.Listen(":8080")
}
