// The project's real API, in a file that sorts after the chi admin router.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type CreateItemRequest struct {
	Name string `json:"name"`
}

type Item struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// createItem exercises the gin-specific extraction that a secondary framework
// must keep: a bound request body, a query parameter, a path parameter, a
// header, and a status-carrying response.
func createItem(c *gin.Context) {
	var req CreateItemRequest
	_ = c.ShouldBindJSON(&req)
	_ = c.Query("filter")
	_ = c.GetHeader("X-Tenant")
	c.JSON(http.StatusCreated, Item{})
}

func getItem(c *gin.Context) {
	_ = c.Param("id")
	c.JSON(http.StatusOK, Item{})
}

func main() {
	_ = adminRouter()

	r := gin.New()
	r.POST("/items", createItem)
	r.GET("/items/:id", getItem)
	_ = r.Run(":8080")
}
