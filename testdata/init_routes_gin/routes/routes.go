// Package routes registers onto main's exported router from its own init(),
// reached only through a blank import.
package routes

import (
	"net/http"

	"github.com/ehabterra/apispec/testdata/init_routes_gin/server"
	"github.com/gin-gonic/gin"
)

type Plugin struct {
	Name string `json:"name"`
}

func init() {
	server.API.GET("/plugins", func(c *gin.Context) {
		c.JSON(http.StatusOK, []Plugin{})
	})
}
