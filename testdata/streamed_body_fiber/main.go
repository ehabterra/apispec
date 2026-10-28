// The streamed-body detection of issue #517, on fiber.
//
// fiber does not use net/http at all: its declaration shorthand is c.Set(k, v)
// and a stream is handed c.Response().BodyWriter(). So its writer types are its
// own — the Ctx, which has Write/WriteString, and fasthttp's Response — where
// gin's and echo's contexts merely expose an http.ResponseWriter.
package main

import (
	"encoding/csv"
	"io"
	"os"

	"github.com/gofiber/fiber/v2"
)

func csvExport(c *fiber.Ctx) error {
	c.Set("Content-Type", "text/csv")
	cw := csv.NewWriter(c.Response().BodyWriter())
	if err := cw.Write([]string{"a", "b"}); err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "boom"})
	}
	cw.Flush()
	return nil
}

func download(c *fiber.Ctx) error {
	f, err := os.Open("/tmp/x.pdf")
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"error": "boom"})
	}
	defer f.Close()
	c.Set("Content-Type", "application/pdf")
	_, _ = io.Copy(c.Response().BodyWriter(), f)
	return nil
}

// Negative: a csv writer over a FILE never reaches the wire.
func internalOnly(c *fiber.Ctx) error {
	f, _ := os.Create("/tmp/out.csv")
	defer f.Close()
	cw := csv.NewWriter(f)
	_ = cw.Write([]string{"x"})
	cw.Flush()
	return c.SendStatus(204)
}

// Raw bytes under a media type declared with c.Set, written by the Ctx itself
// (issues #544, #576).
func rawPDF(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/pdf")
	return c.Send([]byte("%PDF"))
}

// The same under a media type a serializer covers: c.Send writes bytes, so the
// declaration pairs with it as bytes, not as an unresolved YAML document (#576).
func rawHeaderYAML(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/yaml")
	return c.Send([]byte("a: 1"))
}

// fiber's other raw write, c.Write, under the status c.Status sets (#576).
func rawCreatedYAML(c *fiber.Ctx) error {
	c.Set("Content-Type", "application/yaml")
	c.Status(fiber.StatusCreated)
	_, err := c.Write([]byte("a: 1"))
	return err
}

// No declaration: fasthttp does not sniff, so even JSON-looking bytes go out
// as text/plain (issue #596).
func undeclared(c *fiber.Ctx) error {
	return c.Send([]byte(`{"ok":true}`))
}

func main() {
	app := fiber.New()
	app.Get("/export.csv", csvExport)
	app.Get("/file.pdf", download)
	app.Get("/internal", internalOnly)
	app.Get("/raw.pdf", rawPDF)
	app.Get("/raw-header.yaml", rawHeaderYAML)
	app.Get("/raw-created.yaml", rawCreatedYAML)
	app.Get("/undeclared", undeclared)
	_ = app.Listen(":8080")
}
