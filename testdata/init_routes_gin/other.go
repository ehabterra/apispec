package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// A third init in another FILE, same local name again.
func init() {
	g := r.Group("/gamma")
	g.GET("/c", func(c *gin.Context) { c.JSON(http.StatusOK, Item{}) })
}
