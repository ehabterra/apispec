// Package main answers errors by RETURNING them, the fiber way (issue #556).
//
// fiber's default ErrorHandler renders a returned *fiber.Error: the status it
// carries, and its message as plain text.
package main

import "github.com/gofiber/fiber/v2"

// Item is the success body.
type Item struct {
	ID string `json:"id"`
}

func get(c *fiber.Ctx) error {
	if c.Params("id") == "" {
		return fiber.NewError(fiber.StatusNotFound, "not found")
	}
	if c.Params("id") == "taken" {
		// No message: fiber sends the status text.
		return fiber.NewError(fiber.StatusConflict)
	}
	if c.Params("id") == "locked" {
		// A sentinel: a package variable, not a call — NOT yet documented.
		return fiber.ErrForbidden
	}
	return c.JSON(Item{})
}

func main() {
	app := fiber.New()
	app.Get("/items/:id", get)
	_ = app.Listen(":8080")
}
