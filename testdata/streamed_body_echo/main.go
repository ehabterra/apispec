// The streamed-body detection of issue #517, on echo.
//
// echo has no shorthand for the declaration — it reaches net/http's header map
// through c.Response(), so the stdlib ContentTypeWrite entry already matched.
// What was missing is the other half: the declaration counts only when it is
// made on the RESPONSE, and with no ResponseContext.WriterTypeRegexes that
// check could not run, so nothing was claimed.
package main

import (
	"encoding/csv"
	"io"
	"net/http"
	"os"

	"github.com/labstack/echo/v4"
)

func csvExport(c echo.Context) error {
	c.Response().Header().Set("Content-Type", "text/csv")
	cw := csv.NewWriter(c.Response())
	if err := cw.Write([]string{"a", "b"}); err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "boom"})
	}
	cw.Flush()
	return nil
}

func download(c echo.Context) error {
	f, err := os.Open("/tmp/x.pdf")
	if err != nil {
		return c.JSON(http.StatusInternalServerError, map[string]string{"error": "boom"})
	}
	defer f.Close()
	c.Response().Header().Set("Content-Type", "application/pdf")
	_, _ = io.Copy(c.Response(), f)
	return nil
}

// Negative: a csv writer over a FILE never reaches the wire.
func internalOnly(c echo.Context) error {
	f, _ := os.Create("/tmp/out.csv")
	defer f.Close()
	cw := csv.NewWriter(f)
	_ = cw.Write([]string{"x"})
	cw.Flush()
	return c.NoContent(http.StatusNoContent)
}

func main() {
	e := echo.New()
	e.GET("/export.csv", csvExport)
	e.GET("/file.pdf", download)
	e.GET("/internal", internalOnly)
	e.Logger.Fatal(e.Start(":8080"))
}
