package main

import (
	"github.com/ehabterra/swagen/testdata/fiber/payment"
	"github.com/ehabterra/swagen/testdata/fiber/products"
	"github.com/ehabterra/swagen/testdata/fiber/users"
	"github.com/gofiber/fiber/v2"
)

func main() {
	app := fiber.New()

	// Mount user, product, and payment services using a consistent pattern.
	app.Mount("/users", users.Routes())

	// Test case 2: Chained group with Use
	g := app.Group("/products").Use(func(c *fiber.Ctx) error {
		return c.Next()
	})
	products.Routes(g)

	app.Mount("/payment", payment.Routes())

	// Health and API info endpoints can be mounted here or in a separate group/module if desired
	app.Get("/health", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "healthy", "time": "2024-01-01T00:00:00Z"})
	})
	app.Get("/api/info", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{"name": "Fiber API", "version": "1.0.0", "framework": "Fiber"})
	})

	app.Listen(":8080")
}
