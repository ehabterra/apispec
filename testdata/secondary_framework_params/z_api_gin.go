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

// itemStatus answers with a gin.H map. gin.H is only understood through gin's
// own ExternalTypes entry, which SecondaryView used to drop along with the rest
// of the non-pattern config — so this body documented itself as the generic
// "External or unresolved type" placeholder whenever gin was not the primary
// framework (issue #212).
func itemStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func main() {
	_ = adminRouter()

	r := gin.New()
	r.POST("/items", createItem)
	r.GET("/items/:id", getItem)
	r.GET("/items/status", itemStatus)
	_ = r.Run(":8080")
}
