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

func main() {
	app := fiber.New()
	app.Get("/export.csv", csvExport)
	app.Get("/file.pdf", download)
	app.Get("/internal", internalOnly)
	_ = app.Listen(":8080")
}
