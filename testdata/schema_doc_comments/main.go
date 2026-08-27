// Package main documents its types, so the generated schemas can carry those
// comments as descriptions.
package main

import "github.com/gin-gonic/gin"

// Item is a catalogue item.
//
// The second paragraph is kept too, so a longer comment is not truncated.
type Item struct {
	// ID is the unique identifier of the item.
	ID string `json:"id"`
	// Name is the display name.
	Name string `json:"name" validate:"required"`
	// Price is in minor units.
	Price float64 `json:"price"`
	// Secret must not appear at all: json:"-" wins over having a comment.
	Secret string `json:"-"`
	// Owner reuses a documented type; the FIELD's comment must not overwrite
	// the type's own description on the shared component.
	Owner Owner `json:"owner"`
	// Backup is a second field of the same type, with a different comment.
	Backup   Owner  `json:"backup"`
	Quantity int    `json:"quantity"` // trailing comments are collected too
	Undocked string `json:"undocked"`
	// Slug is the first of two fields sharing an inline (non-component) type.
	Slug Code `json:"slug"`
	// Barcode is the second, and must keep its OWN comment.
	Barcode Code `json:"barcode"`
	// Dimensions is an anonymous nested struct, whose schema is ALSO registered
	// as a component — the one path where the field schema is genuinely shared.
	Dimensions struct {
		// Width is in millimetres.
		Width int `json:"width"`
	} `json:"dimensions"`
}

// Code is a named string, which resolves inline rather than to a component.
type Code string

// Owner is the party responsible for an item.
type Owner struct {
	// Email is the contact address.
	Email string `json:"email"`
}

func getItem(c *gin.Context) { c.JSON(200, Item{}) }

func main() {
	r := gin.New()
	r.GET("/items/:id", getItem)
	_ = r.Run(":8080")
}
