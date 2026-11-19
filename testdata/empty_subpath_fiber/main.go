// An empty registration path under a group is the group's own path (issue
// #609): fiber serves `items.Get("", h)` at /api/items.
package main

import "github.com/gofiber/fiber/v2"

type Item struct {
	ID int `json:"id"`
}

func listItems(c *fiber.Ctx) error  { return c.JSON([]Item{}) }
func createItem(c *fiber.Ctx) error { return c.Status(fiber.StatusCreated).JSON(Item{}) }
func getItem(c *fiber.Ctx) error    { return c.JSON(Item{}) }

func main() {
	app := fiber.New()
	api := app.Group("/api")
	items := api.Group("/items")
	items.Get("", listItems)
	items.Post("", createItem)
	items.Get("/:id", getItem)
	app.Listen(":8080")
}
