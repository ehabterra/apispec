// Fixture: the echo form of a group created inside a helper and returned to
// the caller (issue #633):
//
//	/ident/x          g := e.Group("/ident"); return g
//	/direct/x         return e.Group("/direct")
//	/public/v/a       Version(pub) and Version(adm): one helper, two parents,
//	/admin/v/b        no route leaking into the other
//	/kept/same        Same(g) returns its parameter (negative: no doubling)
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

func h(c echo.Context) error { return c.NoContent(http.StatusOK) }

func Ident(e *echo.Echo) *echo.Group { g := e.Group("/ident"); return g }

func Direct(e *echo.Echo) *echo.Group { return e.Group("/direct") }

func Version(parent *echo.Group) *echo.Group { return parent.Group("/v") }

func Same(g *echo.Group) *echo.Group { return g }

func main() {
	e := echo.New()

	ident := Ident(e)
	ident.GET("/x", h)

	direct := Direct(e)
	direct.GET("/x", h)

	pub := e.Group("/public")
	adm := e.Group("/admin")
	a := Version(pub)
	a.GET("/a", h)
	b := Version(adm)
	b.GET("/b", h)

	kept := e.Group("/kept")
	same := Same(kept)
	same.GET("/same", h)

	_ = e.Start(":8080")
}
