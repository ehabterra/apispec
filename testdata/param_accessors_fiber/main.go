// Package main calls every parameter accessor fiber's own documentation leads
// with. fiber spells the request-header read `Get`, and the config had no header
// pattern at all — so no fiber handler could produce an `in: header` parameter,
// and a required tenant or API-version header was invisible to every client
// generated from the document (issue #365).
package main

import "github.com/gofiber/fiber/v2"

type Item struct {
	ID string `json:"id"`
}

func getItem(c *fiber.Ctx) error {
	_ = c.Params("id")
	_ = c.Get("X-Tenant")
	_ = c.Query("q")
	_ = c.Cookies("session")
	_ = c.FormValue("field")
	return c.JSON(Item{})
}

func main() {
	app := fiber.New()
	app.Get("/items/:id", getItem)
	_ = app.Listen(":8080")
}
