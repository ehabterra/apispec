// A router embedded in a user struct promotes the framework's methods, so the
// call is written against the user type but runs the framework's (issue #643).
// Every route below must keep the prefix of the group it is registered on.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Reply struct {
	Name string `json:"name"`
}

type Item struct {
	ID string `json:"id"`
}

// Server embeds the framework router: GET and Group are echo's, promoted.
type Server struct{ *echo.Echo }

// Outer embeds Server, so echo's methods are promoted through two fields.
type Outer struct{ Server }

// Shadow declares its own Group, which shadows echo's. A call to it is a call
// to Shadow.Group, not to echo's, and must not be matched as a mount.
type Shadow struct{ *echo.Echo }

func (s *Shadow) Group(prefix string, m ...echo.MiddlewareFunc) *echo.Group {
	return s.Echo.Group("/fixed", m...)
}

// Base carries the response helper; Ctx promotes it, so handlers write
// ctx.JSON against Ctx while the body runs in Base.JSON.
type Base struct{ c echo.Context }

func (b *Base) JSON(code int, v any) error { return b.c.JSON(code, v) }

type Ctx struct{ *Base }

func main() {
	e := echo.New()

	// Promoted through one embedded field.
	srv := &Server{Echo: e}
	srv.GET("/direct", handleDirect)
	srv.Group("/grp").GET("/inline", handleInline)
	g := srv.Group("/var")
	g.GET("/item", handleItem)
	registerSrv(srv.Group("/arg"))

	// Promoted through two embedded fields.
	o := &Outer{Server: Server{Echo: e}}
	o.Group("/deep").GET("/x", handleDirect)

	// Shadowed: the embedding type's own Group.
	s := &Shadow{Echo: e}
	s.Group("/ignored").GET("/s", handleDirect)

	e.Logger.Fatal(e.Start(":8080"))
}

func registerSrv(g *echo.Group) { g.GET("/r", handleItem) }

func handleDirect(c echo.Context) error { return c.JSON(http.StatusOK, Reply{Name: "d"}) }
func handleInline(c echo.Context) error { return c.JSON(http.StatusOK, Reply{Name: "i"}) }

// handleItem answers through the promoted helper; its body is Base.JSON's.
func handleItem(c echo.Context) error {
	ctx := &Ctx{Base: &Base{c: c}}
	return ctx.JSON(http.StatusOK, Item{ID: "1"})
}
