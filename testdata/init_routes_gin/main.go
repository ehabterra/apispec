package main

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/init_routes_gin/server"
	"github.com/gin-gonic/gin"

	_ "github.com/ehabterra/apispec/testdata/init_routes_gin/routes"
)

type Item struct {
	ID string `json:"id"`
}

type Health struct {
	OK bool `json:"ok"`
}

// A package-level router populated from init: the routes sit below init, not
// below main, which only starts the server.
var r = gin.New()

func getItem(c *gin.Context) { c.JSON(http.StatusOK, Item{}) }

func init() {
	r.GET("/items/:id", getItem)
	v1 := r.Group("/v1")
	v1.GET("/health", func(c *gin.Context) { c.JSON(http.StatusOK, Health{}) })
}

func main() {
	r.GET("/from-main", getItem)
	go func() { _ = server.Engine.Run(":8081") }()
	_ = r.Run(":8080")
}
