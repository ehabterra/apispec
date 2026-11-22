// Fixture: control flow inside a METHOD handler answers exactly as it does
// inside the identical FUNCTION handler (issue #617), on gin.
//
// Each shape is written twice — /fn/... served by a function, /m/... by a
// method on H — and the two must document the same responses. See
// generator/testdata_method_blocks_test.go.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Item struct {
	ID string `json:"id"`
}

type Err struct {
	Message string `json:"message"`
}

type H struct{}

func main() {
	r := gin.Default()
	h := &H{}

	r.POST("/fn/create", create)
	r.POST("/m/create", h.Create)

	r.GET("/fn/guard", guard)
	r.GET("/m/guard", h.Guard)

	r.GET("/fn/alternatives", alternatives)
	r.GET("/m/alternatives", h.Alternatives)

	r.GET("/fn/sequential", sequential)
	r.GET("/m/sequential", h.Sequential)

	r.GET("/fn/branchstatus", branchStatus)
	r.GET("/m/branchstatus", h.BranchStatus)

	r.Run(":8080")
}

// create is the early-return error shape.
func create(c *gin.Context) {
	if c.Query("bad") != "" {
		c.JSON(http.StatusBadRequest, Err{Message: "bad"})
		return
	}
	c.JSON(http.StatusCreated, Item{ID: "1"})
}

func (h *H) Create(c *gin.Context) {
	if c.Query("bad") != "" {
		c.JSON(http.StatusBadRequest, Err{Message: "bad"})
		return
	}
	c.JSON(http.StatusCreated, Item{ID: "1"})
}

// guard's 400 is bodyless: the success body below must not carry it.
func guard(c *gin.Context) {
	if c.Query("id") == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	c.JSON(http.StatusOK, Item{ID: "1"})
}

func (h *H) Guard(c *gin.Context) {
	if c.Query("id") == "" {
		c.Status(http.StatusBadRequest)
		return
	}
	c.JSON(http.StatusOK, Item{ID: "1"})
}

// alternatives sends one status from both arms of an if/else, each with its
// own type: the 202 really is either.
func alternatives(c *gin.Context) {
	if c.Query("detail") != "" {
		c.JSON(http.StatusAccepted, Item{ID: "1"})
	} else {
		c.JSON(http.StatusAccepted, Err{Message: "queued"})
	}
}

func (h *H) Alternatives(c *gin.Context) {
	if c.Query("detail") != "" {
		c.JSON(http.StatusAccepted, Item{ID: "1"})
	} else {
		c.JSON(http.StatusAccepted, Err{Message: "queued"})
	}
}

// sequential sends two bodies under one status, one after the other. Both run,
// so they are not alternatives — today documented as an anyOf all the same;
// see the change-detector in the structural test.
func sequential(c *gin.Context) {
	c.JSON(http.StatusCreated, Item{ID: "1"})
	c.JSON(http.StatusCreated, Err{Message: "trailer"})
}

func (h *H) Sequential(c *gin.Context) {
	c.JSON(http.StatusCreated, Item{ID: "1"})
	c.JSON(http.StatusCreated, Err{Message: "trailer"})
}

// branchStatus picks its status in a branch: the 200 written above the `if`
// still reaches the call whenever the branch is not taken.
func branchStatus(c *gin.Context) {
	status := http.StatusOK
	if c.Query("async") != "" {
		status = http.StatusAccepted
	}
	c.JSON(status, Item{ID: "1"})
}

func (h *H) BranchStatus(c *gin.Context) {
	status := http.StatusOK
	if c.Query("async") != "" {
		status = http.StatusAccepted
	}
	c.JSON(status, Item{ID: "1"})
}
