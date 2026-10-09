package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Item struct {
	ID string `json:"id"`
}

func short(c *gin.Context) {
	code := http.StatusConflict
	c.JSON(code, Item{})
}

// A status setter, then a body: the setter's variable must resolve too.
func status(c *gin.Context) {
	code := http.StatusAccepted
	c.Status(code)
	_, _ = c.Writer.Write([]byte("ok"))
}

func abort(c *gin.Context) {
	code := http.StatusForbidden
	c.AbortWithStatusJSON(code, Item{})
}

func main() {
	r := gin.New()
	r.GET("/short", short)
	r.GET("/status", status)
	r.GET("/abort", abort)
	_ = r.Run(":8080")
}
