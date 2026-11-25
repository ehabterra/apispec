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

// gin's c.Status only records the status: the headers go out with the first
// body write, so a Content-Type set after it is still the body's.
func statusThenHeader(c *gin.Context) {
	c.Status(http.StatusConflict)
	c.Header("Content-Type", "application/problem+json")
	c.JSON(http.StatusConflict, Problem{Title: "conflict"})
}

type responder struct{}

// The same order inside a responder: every call site's body takes the
// declaration.
func (responder) problem(c *gin.Context, code int, v any) {
	c.Status(code)
	c.Header("Content-Type", "application/problem+json")
	c.JSON(code, v)
}

type api struct{ out responder }

func (a *api) statusHelper(c *gin.Context) {
	a.out.problem(c, http.StatusGone, Problem{Title: "gone"})
}

// AbortWithStatus writes the headers at once: a Content-Type set after it
// never reaches the client.
func abortThenHeader(c *gin.Context) {
	c.AbortWithStatus(http.StatusLocked)
	c.Header("Content-Type", "application/problem+json")
	c.JSON(http.StatusLocked, Problem{Title: "locked"})
}

func main() {
	r := gin.New()
	r.GET("/branch-status", branchStatus)
	r.GET("/residue-status", residueStatus)
	r.GET("/undeclared", undeclared)
	r.GET("/status-then-header", statusThenHeader)
	r.GET("/status-helper", (&api{}).statusHelper)
	r.GET("/abort-then-header", abortThenHeader)
	_ = r.Run(":8080")
}
