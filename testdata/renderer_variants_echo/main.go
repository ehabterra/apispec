package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Item struct {
	ID string `json:"id" xml:"id"`
}

func jsonPlain(c echo.Context) error  { return c.JSON(http.StatusOK, Item{}) }
func jsonPretty(c echo.Context) error { return c.JSONPretty(http.StatusOK, Item{}, "  ") }
func jsonBlob(c echo.Context) error   { return c.JSONBlob(http.StatusOK, []byte(`{}`)) }
func jsonp(c echo.Context) error      { return c.JSONP(http.StatusOK, "cb", Item{}) }
func jsonpBlob(c echo.Context) error  { return c.JSONPBlob(http.StatusOK, "cb", []byte(`{}`)) }
func xmlPlain(c echo.Context) error   { return c.XML(http.StatusOK, Item{}) }
func xmlPretty(c echo.Context) error  { return c.XMLPretty(http.StatusOK, Item{}, " ") }
func xmlBlob(c echo.Context) error    { return c.XMLBlob(http.StatusOK, []byte("<a/>")) }
func html(c echo.Context) error       { return c.HTML(http.StatusOK, "<p>hi</p>") }
func htmlBlob(c echo.Context) error   { return c.HTMLBlob(http.StatusOK, []byte("<p/>")) }
func str(c echo.Context) error        { return c.String(http.StatusOK, "ok") }
func blob(c echo.Context) error       { return c.Blob(http.StatusOK, "application/pdf", []byte("%PDF")) }
func render(c echo.Context) error     { return c.Render(http.StatusOK, "page", Item{}) }
func file(c echo.Context) error       { return c.File("a.pdf") }
func attach(c echo.Context) error     { return c.Attachment("a.pdf", "a.pdf") }
func inline(c echo.Context) error     { return c.Inline("a.pdf", "a.pdf") }
func redirect(c echo.Context) error   { return c.Redirect(http.StatusFound, "/x") }
func noContent(c echo.Context) error  { return c.NoContent(http.StatusNoContent) }

// The first WriteHeader commits echo's response: the file goes out under 201.
func fileCreated(c echo.Context) error {
	c.Response().WriteHeader(http.StatusCreated)
	return c.File("a.pdf")
}

func main() {
	e := echo.New()
	e.GET("/json", jsonPlain)
	e.GET("/json-pretty", jsonPretty)
	e.GET("/json-blob", jsonBlob)
	e.GET("/jsonp", jsonp)
	e.GET("/jsonp-blob", jsonpBlob)
	e.GET("/xml", xmlPlain)
	e.GET("/xml-pretty", xmlPretty)
	e.GET("/xml-blob", xmlBlob)
	e.GET("/html", html)
	e.GET("/html-blob", htmlBlob)
	e.GET("/string", str)
	e.GET("/blob", blob)
	e.GET("/render", render)
	e.GET("/file", file)
	e.GET("/attachment", attach)
	e.GET("/inline", inline)
	e.GET("/redirect", redirect)
	e.GET("/no-content", noContent)
	e.GET("/file-created", fileCreated)
	_ = e.Start(":8080")
}
