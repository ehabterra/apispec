// Package main binds whole structs with fiber's parsers (issue #355). fiber
// reads an untagged field under its Go name.
package main

import "github.com/gofiber/fiber/v2"

type Item struct {
	ID string `json:"id"`
}

type ListQuery struct {
	Page   int      `query:"page"`
	Tags   []string `query:"tag"`
	Cursor string   // untagged: fiber binds it under the Go name
	Skip   string   `query:"-"`
}

type ItemParams struct {
	ID int `params:"id"`
}

type TraceHeader struct {
	RequestID string `reqHeader:"X-Request-ID"`
}

func listItems(c *fiber.Ctx) error {
	var q ListQuery
	if err := c.QueryParser(&q); err != nil {
		return err
	}
	return c.JSON([]Item{})
}

func getItem(c *fiber.Ctx) error {
	var p ItemParams
	if err := c.ParamsParser(&p); err != nil {
		return err
	}
	var h TraceHeader
	_ = c.ReqHeaderParser(&h)
	return c.JSON(Item{})
}

func main() {
	app := fiber.New()
	app.Get("/items", listItems)
	app.Get("/items/:id", getItem)
	_ = app.Listen(":3000")
}
