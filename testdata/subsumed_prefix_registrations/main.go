// Fixture: one handler registered at two prefixes where one prefix extends the
// other (issue #614).
//
// dropSubsumedMountPrefixes removes the partial-mount echoes a nested mount
// produces: one registration call reached through several traversal contexts
// comes out at every subset of its mount chain, and only the most-mounted form
// is real. Judged by mount segments alone, two SEPARATE registrations whose
// prefixes happen to extend each other look exactly like that echo, so the
// shorter one — a real endpoint — was dropped without a word.
//
// Three shapes side by side:
//
//	/:owner/:repo/attachments/:uuid                 two registration calls at
//	/:owner/:repo/issues/:index/attachments/:uuid   extending prefixes: BOTH real
//
//	/api/users/:id   one registration in a helper, handed a nested group; the
//	                 traversal also reaches it un-prefixed — only this survives
//
//	/a/items         ONE registration call in a helper, invoked with two
//	/a/b/items       genuine extending prefixes (the probe)
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Attachment struct {
	UUID string `json:"uuid"`
	Name string `json:"name"`
}

type User struct {
	ID string `json:"id"`
}

type Item struct {
	SKU string `json:"sku"`
}

func getAttachment(c *gin.Context) {
	uuid := c.Param("uuid")
	c.JSON(http.StatusOK, Attachment{UUID: uuid})
}

func getUser(c *gin.Context) {
	c.JSON(http.StatusOK, User{ID: c.Param("id")})
}

func listItems(c *gin.Context) {
	c.JSON(http.StatusOK, []Item{})
}

// registerUsers registers onto whatever group it is handed.
func registerUsers(g *gin.RouterGroup) {
	g.GET("/:id", getUser)
}

// registerItems is called twice, with prefixes /a and /a/b.
func registerItems(g *gin.RouterGroup) {
	g.GET("/items", listItems)
}

func main() {
	r := gin.New()

	// Two registrations, extending prefixes.
	repo := r.Group("/:owner/:repo")
	repo.GET("/attachments/:uuid", getAttachment)
	repo.Group("/issues/:index").GET("/attachments/:uuid", getAttachment)

	// A nested group handed to a helper.
	api := r.Group("/api")
	registerUsers(api.Group("/users"))

	// One registration site, two genuine prefixes.
	a := r.Group("/a")
	registerItems(a)
	registerItems(a.Group("/b"))

	_ = r.Run(":8080")
}
