package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Two inits in one FILE, same local name, different prefixes.
func init() {
	g := r.Group("/alpha")
	g.GET("/a", func(c *gin.Context) { c.JSON(http.StatusOK, Item{}) })
}

func init() {
	g := r.Group("/beta")
	g.GET("/b", func(c *gin.Context) { c.JSON(http.StatusOK, Health{}) })
}
