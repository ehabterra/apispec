// Package main exercises fiber's route-constraint syntax, where a constraint
// follows the parameter name in angle brackets and is not part of the URL.
package main

import "github.com/gofiber/fiber/v2"

// Item is what every handler here returns.
type Item struct {
	ID string `json:"id"`
}

func main() {
	app := fiber.New()

	app.Get("/items/:id<int>", getItem)
	app.Get("/users/:uid<guid>", getUser)
	// A constraint taking an argument, whose parens may contain anything.
	app.Get("/events/:day<datetime(2006-01-02)>", getEvent)
	// Several constraints chained with ';'.
	app.Get("/pages/:n<min(1);max(500)>", getPage)
	// An unconstrained parameter alongside, which must be unaffected.
	app.Get("/plain/:name", getPlain)

	_ = app.Listen(":8080")
}

func getItem(c *fiber.Ctx) error  { return c.JSON(Item{ID: c.Params("id")}) }
func getUser(c *fiber.Ctx) error  { return c.JSON(Item{ID: c.Params("uid")}) }
func getEvent(c *fiber.Ctx) error { return c.JSON(Item{ID: c.Params("day")}) }
func getPage(c *fiber.Ctx) error  { return c.JSON(Item{ID: c.Params("n")}) }
func getPlain(c *fiber.Ctx) error { return c.JSON(Item{ID: c.Params("name")}) }
