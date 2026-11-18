// Fixture for issue #600: gin validates a request body with the `binding`
// struct-tag key, not `validate`, so a body declared the gin way must still
// document its required fields and constraints.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Address is nested: its schema is a component of its own, and its `binding`
// rules must reach that component too.
type Address struct {
	City string `json:"city" binding:"required"`
	Zip  string `json:"zip" binding:"len=5"`
}

// CreateItem is the issue's reproduction, plus the shapes around it.
type CreateItem struct {
	// The gin way.
	Name string `json:"name" binding:"required,min=3"`
	// The standard key still works beside it.
	Email string `json:"email" validate:"required,email"`
	// Both keys on one field: the rules of each apply.
	Code string `json:"code" binding:"required" validate:"max=8"`
	// Numeric rules.
	Quantity int `json:"quantity" binding:"min=1,max=100"`
	// Container rules before `dive`, element rules after it.
	Tags []string `json:"tags" binding:"max=5,dive,min=2"`
	// `dive` under one key must not swallow the other key's container rules:
	// `required` is the slice's, not each element's.
	Labels  []string `json:"labels" validate:"dive,max=10" binding:"required"`
	Address Address  `json:"address" binding:"required"`
	// Not validated at all.
	Note string `json:"note"`
}

func create(c *gin.Context) {
	var req CreateItem
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, req)
}

func main() {
	r := gin.Default()
	r.POST("/items", create)
	_ = r.Run()
}
