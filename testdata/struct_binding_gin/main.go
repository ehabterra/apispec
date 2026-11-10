// Package main binds whole structs the way gin's documentation teaches: the
// struct's tags, not a call argument, name the parameters (issue #355).
package main

import "github.com/gin-gonic/gin"

type Item struct {
	ID string `json:"id"`
}

// Pagination is embedded untagged, so gin promotes its fields.
type Pagination struct {
	Page  int `form:"page" binding:"required,min=1"`
	Limit int `form:"limit" binding:"max=100"`
}

// Filter is a struct-typed field: not one parameter.
type Filter struct {
	Owner string `form:"owner"`
}

type ListQuery struct {
	Pagination
	Sort    string   `form:"sort"`
	Tags    []string `form:"tag"`
	Since   *int64   `form:"since"`
	Cursor  string   // untagged: gin binds it under the Go name
	Skipped string   `form:"-"`
	Filter  Filter   `form:"filter"`
	hidden  string   //nolint:unused // unexported: never bound
}

type ItemURI struct {
	ID    string `uri:"id" binding:"required"`
	Extra string `uri:"extra"` // the route has no :extra segment
}

type TraceHeader struct {
	RequestID string `header:"X-Request-ID" binding:"required"`
}

type WrappedQuery struct {
	Q string `form:"q"`
}

func listItems(c *gin.Context) {
	var q ListQuery
	if err := c.ShouldBindQuery(&q); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, []Item{})
}

func getItem(c *gin.Context) {
	var u ItemURI
	if err := c.ShouldBindUri(&u); err != nil {
		c.JSON(400, gin.H{"error": err.Error()})
		return
	}
	var h TraceHeader
	_ = c.ShouldBindHeader(&h)
	c.JSON(200, Item{})
}

// bindQuery is a house wrapper: the bound type is the caller's.
func bindQuery(c *gin.Context, v any) error {
	return c.ShouldBindQuery(v)
}

func search(c *gin.Context) {
	var q WrappedQuery
	_ = bindQuery(c, &q)
	c.JSON(200, []Item{})
}

type MixedQuery struct {
	Page int    `form:"page" binding:"required"`
	Sort string `form:"sort"`
}

// mixed reads the query both ways: a bound struct and literal accessors, with
// `page` read by both. It is one parameter, not two, and the accessor is read
// FIRST so the struct's type and validation must win on merit, not on order.
func mixed(c *gin.Context) {
	_ = c.Query("page")
	var q MixedQuery
	_ = c.ShouldBindQuery(&q)
	_ = c.GetHeader("X-Tenant")
	c.JSON(200, []Item{})
}

// untyped binds into a value whose type is not known: nothing is documented.
func untyped(c *gin.Context) {
	var v any
	_ = c.ShouldBindQuery(&v)
	c.JSON(200, Item{})
}

func main() {
	r := gin.Default()
	r.GET("/items", listItems)
	r.GET("/items/:id", getItem)
	r.GET("/search", search)
	r.GET("/untyped", untyped)
	r.GET("/mixed", mixed)
	_ = r.Run()
}
