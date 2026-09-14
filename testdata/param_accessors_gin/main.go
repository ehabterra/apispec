// Package main calls every parameter accessor gin's own documentation leads
// with. The lists in each framework config drifted apart, so the same source was
// detected on one router and silently dropped on the next: gin had no cookie
// accessor, no comma-ok or multi-value variants, and no `Params.ByName`
// (issue #365).
package main

import "github.com/gin-gonic/gin"

type Item struct {
	ID string `json:"id"`
}

func getItem(c *gin.Context) {
	_ = c.Param("id")
	_, _ = c.Cookie("session")
	_ = c.GetHeader("X-Tenant")
	_ = c.QueryArray("tag")
	_ = c.DefaultQuery("page", "1")
	_, _ = c.GetQuery("q")
	_ = c.Params.ByName("altid")
	_ = c.PostForm("field")
	_, _ = c.GetPostForm("optfield")
	_ = c.PostFormArray("multifield")
	c.JSON(200, Item{})
}

func main() {
	r := gin.Default()
	r.GET("/items/:id", getItem)
	_ = r.Run(":8080")
}
