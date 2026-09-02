// Package main is the fiber half of issue #370: keyauth.Config carries the same
// KeyLookup grammar echo uses, and a custom header name was documented as
// `Authorization` — the library default — rather than the one the middleware
// actually reads.
package main

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/keyauth"
)

func items(c *fiber.Ctx) error  { return c.JSON(fiber.Map{}) }
func health(c *fiber.Ctx) error { return c.SendString("ok") }

func main() {
	app := fiber.New()

	api := app.Group("/api")
	api.Use(keyauth.New(keyauth.Config{
		KeyLookup: "header:X-API-Key",
	}))
	api.Get("/items", items)

	app.Get("/health", health) // open
	_ = app.Listen(":8080")
}
