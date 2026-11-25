// Issue #645 on gin: a responder method writing through the context's
// `c.Writer` field documents what the same body written by a function does.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Base struct {
	c *gin.Context
}

// PlainText declares text/plain on gin's writer and writes the bytes.
func (b *Base) PlainText(status int, text string) {
	b.c.Writer.Header().Set("Content-Type", "text/plain;charset=utf-8")
	b.c.Writer.WriteHeader(status)
	_, _ = b.c.Writer.Write([]byte(text))
}

func plainText(c *gin.Context, status int, text string) {
	c.Writer.Header().Set("Content-Type", "text/plain;charset=utf-8")
	c.Writer.WriteHeader(status)
	_, _ = c.Writer.Write([]byte(text))
}

func methodText(c *gin.Context) {
	b := &Base{c: c}
	if c.Query("id") == "" {
		b.PlainText(http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func funcText(c *gin.Context) {
	if c.Query("id") == "" {
		plainText(c, http.StatusInternalServerError, "failed")
		return
	}
	c.Status(http.StatusNoContent)
}

func main() {
	r := gin.New()
	r.GET("/method/text", methodText)
	r.GET("/func/text", funcText)
	_ = r.Run(":8080")
}
