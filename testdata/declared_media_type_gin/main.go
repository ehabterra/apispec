package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Problem struct {
	Title string `json:"title"`
}

func lookup() int { return http.StatusTeapot }

// gin's JSON renderer sets Content-Type only when the handler set none, so the
// declaration is the body's media type at every status the branches assign.
func branchStatus(c *gin.Context) {
	c.Header("Content-Type", "application/problem+json")
	code := http.StatusBadRequest
	if c.Query("strict") != "" {
		code = http.StatusUnprocessableEntity
	}
	c.JSON(code, Problem{Title: "invalid"})
}

// One branch assigns a status nothing can read: the fan-out's residue copy
// carries the declaration's media type like the resolved one.
func residueStatus(c *gin.Context) {
	c.Header("Content-Type", "application/problem+json")
	code := http.StatusBadRequest
	if c.Query("other") != "" {
		code = lookup()
	}
	c.JSON(code, Problem{Title: "invalid"})
}

// No declaration: the fanned-out statuses stay under the default.
func undeclared(c *gin.Context) {
	code := http.StatusBadRequest
	if c.Query("strict") != "" {
		code = http.StatusUnprocessableEntity
	}
	c.JSON(code, Problem{Title: "invalid"})
}

func main() {
	r := gin.New()
	r.GET("/branch-status", branchStatus)
	r.GET("/residue-status", residueStatus)
	r.GET("/undeclared", undeclared)
	_ = r.Run(":8080")
}
