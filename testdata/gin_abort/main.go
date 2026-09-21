// Package main answers errors through gin's abort family (issue #551).
//
// AbortWithStatusJSON / AbortWithStatus / AbortWithError write a status (and,
// for the JSON form, a body) exactly as gin's renderers do, under names no
// renderer pattern matched — so every error a gin handler or middleware
// aborted with went undocumented, and the operation showed only its success.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Item is the success body.
type Item struct {
	ID string `json:"id"`
}

// ErrorBody is the error body.
type ErrorBody struct {
	Error string `json:"error"`
}

// abort is a house error helper, the shape real projects route every error
// through: the status reaches AbortWithStatusJSON as a PARAMETER.
func abort(c *gin.Context, code int, msg string) {
	c.AbortWithStatusJSON(code, ErrorBody{Error: msg})
}

func withJSON(c *gin.Context) {
	if c.Param("id") == "" {
		c.AbortWithStatusJSON(http.StatusNotFound, ErrorBody{Error: "not found"})
		return
	}
	c.JSON(http.StatusOK, Item{})
}

func withStatus(c *gin.Context) {
	if c.Param("id") == "" {
		c.AbortWithStatus(http.StatusForbidden)
		return
	}
	c.JSON(http.StatusOK, Item{})
}

func withError(c *gin.Context) {
	if c.Param("id") == "" {
		// The error is attached for middleware; gin writes only the status.
		_ = c.AbortWithError(http.StatusUnprocessableEntity, http.ErrNoCookie)
		return
	}
	c.JSON(http.StatusOK, Item{})
}

func viaHelper(c *gin.Context) {
	if c.Param("id") == "" {
		abort(c, http.StatusConflict, "conflict")
		return
	}
	c.JSON(http.StatusOK, Item{})
}

func main() {
	r := gin.Default()
	r.GET("/json/:id", withJSON)
	r.GET("/status/:id", withStatus)
	r.GET("/error/:id", withError)
	r.GET("/helper/:id", viaHelper)
	_ = r.Run(":8080")
}
