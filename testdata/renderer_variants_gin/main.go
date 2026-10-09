package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Item struct {
	ID string `json:"id" xml:"id" yaml:"id" toml:"id"`
}

func jsonPlain(c *gin.Context)    { c.JSON(http.StatusOK, Item{}) }
func indentedJSON(c *gin.Context) { c.IndentedJSON(http.StatusOK, Item{}) }
func secureJSON(c *gin.Context)   { c.SecureJSON(http.StatusOK, Item{}) }
func asciiJSON(c *gin.Context)    { c.AsciiJSON(http.StatusOK, Item{}) }
func pureJSON(c *gin.Context)     { c.PureJSON(http.StatusOK, Item{}) }
func jsonp(c *gin.Context)        { c.JSONP(http.StatusOK, Item{}) }
func xmlPlain(c *gin.Context)     { c.XML(http.StatusOK, Item{}) }
func yamlPlain(c *gin.Context)    { c.YAML(http.StatusOK, Item{}) }
func tomlPlain(c *gin.Context)    { c.TOML(http.StatusOK, Item{}) }
func str(c *gin.Context)          { c.String(http.StatusOK, "ok %d", 1) }
func html(c *gin.Context)         { c.HTML(http.StatusOK, "page.tmpl", Item{}) }
func data(c *gin.Context)         { c.Data(http.StatusOK, "application/pdf", []byte("%PDF")) }
func file(c *gin.Context)         { c.File("a.pdf") }
func fileAttachment(c *gin.Context) {
	c.FileAttachment("a.pdf", "a.pdf")
}
func fileFromFS(c *gin.Context) { c.FileFromFS("a.pdf", http.Dir(".")) }
func redirect(c *gin.Context)   { c.Redirect(http.StatusFound, "/x") }
func status(c *gin.Context)     { c.Status(http.StatusNoContent) }

// gin's writer lets the file server's WriteHeader(200) replace the 201.
func fileCreated(c *gin.Context) {
	c.Status(http.StatusCreated)
	c.File("a.pdf")
}

func main() {
	r := gin.New()
	r.GET("/json", jsonPlain)
	r.GET("/indented-json", indentedJSON)
	r.GET("/secure-json", secureJSON)
	r.GET("/ascii-json", asciiJSON)
	r.GET("/pure-json", pureJSON)
	r.GET("/jsonp", jsonp)
	r.GET("/xml", xmlPlain)
	r.GET("/yaml", yamlPlain)
	r.GET("/toml", tomlPlain)
	r.GET("/string", str)
	r.GET("/html", html)
	r.GET("/data", data)
	r.GET("/file", file)
	r.GET("/file-attachment", fileAttachment)
	r.GET("/file-from-fs", fileFromFS)
	r.GET("/redirect", redirect)
	r.GET("/status", status)
	r.GET("/file-created", fileCreated)
	_ = r.Run(":8080")
}
