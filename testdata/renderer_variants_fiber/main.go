package main

import (
	"github.com/gofiber/fiber/v2"
)

type Item struct {
	ID string `json:"id" xml:"id"`
}

func jsonPlain(c *fiber.Ctx) error  { return c.JSON(Item{}) }
func jsonp(c *fiber.Ctx) error      { return c.JSONP(Item{}, "cb") }
func xmlPlain(c *fiber.Ctx) error   { return c.XML(Item{}) }
func str(c *fiber.Ctx) error        { return c.SendString("ok") }
func sendFile(c *fiber.Ctx) error   { return c.SendFile("a.pdf") }
func download(c *fiber.Ctx) error   { return c.Download("a.pdf", "a.pdf") }
func redirect(c *fiber.Ctx) error   { return c.Redirect("/x", fiber.StatusFound) }
func redirectD(c *fiber.Ctx) error  { return c.Redirect("/x") }
func sendStatus(c *fiber.Ctx) error { return c.SendStatus(fiber.StatusNoContent) }
func render(c *fiber.Ctx) error     { return c.Render("page", Item{}) }
func notFound(c *fiber.Ctx) error   { return c.SendStatus(fiber.StatusNotFound) }

// A status passed but not statically known: not fiber's omitted-status 302.
func redirectComputed(c *fiber.Ctx) error { return c.Redirect("/x", statusFor(c)) }

func statusFor(c *fiber.Ctx) int {
	if c.Query("permanent") != "" {
		return fiber.StatusMovedPermanently
	}
	return fiber.StatusTemporaryRedirect
}

// SendFile keeps a status the handler set first.
func sendFileCreated(c *fiber.Ctx) error { return c.Status(fiber.StatusCreated).SendFile("a.pdf") }

func main() {
	app := fiber.New()
	app.Get("/json", jsonPlain)
	app.Get("/jsonp", jsonp)
	app.Get("/xml", xmlPlain)
	app.Get("/string", str)
	app.Get("/send-file", sendFile)
	app.Get("/download", download)
	app.Get("/redirect", redirect)
	app.Get("/redirect-default", redirectD)
	app.Get("/send-status", sendStatus)
	app.Get("/render", render)
	app.Get("/not-found", notFound)
	app.Get("/redirect-computed", redirectComputed)
	app.Get("/send-file-created", sendFileCreated)
	_ = app.Listen(":8080")
}
