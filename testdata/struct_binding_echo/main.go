// Package main binds whole structs with echo (issue #355). echo's Bind fills
// path params always and the query string only for GET, DELETE and HEAD, and
// binds nothing into an untagged field.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Item struct {
	ID string `json:"id"`
}

type GetItemRequest struct {
	ID     string `param:"id"`
	Fields string `query:"fields" validate:"required"`
	Cursor string // untagged: echo binds nothing here
}

type UpdateItemRequest struct {
	ID     string `param:"id"`
	DryRun bool   `query:"dry_run"` // a POST: echo's Bind skips the query
	Name   string `json:"name"`
}

type ListQuery struct {
	Page int `query:"page"`
}

type TraceHeader struct {
	RequestID string `header:"X-Request-ID"`
}

func getItem(c echo.Context) error {
	var req GetItemRequest
	if err := c.Bind(&req); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, Item{})
}

func updateItem(c echo.Context) error {
	var req UpdateItemRequest
	if err := c.Bind(&req); err != nil {
		return err
	}
	return c.JSON(http.StatusOK, Item{})
}

func listItems(c echo.Context) error {
	var q ListQuery
	binder := &echo.DefaultBinder{}
	if err := binder.BindQueryParams(c, &q); err != nil {
		return err
	}
	var h TraceHeader
	_ = binder.BindHeaders(c, &h)
	return c.JSON(http.StatusOK, []Item{})
}

func main() {
	e := echo.New()
	e.GET("/items", listItems)
	e.GET("/items/:id", getItem)
	e.POST("/items/:id", updateItem)
	e.Logger.Fatal(e.Start(":8080"))
}
