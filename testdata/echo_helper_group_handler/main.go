// A helper that receives both the group and the handler (issue #615), on echo:
// the same shape as gin_helper_group_handler, through a different framework's
// group and handler types.
package main

import "github.com/labstack/echo/v4"

// Place is the body every handler in this fixture returns.
type Place struct {
	Name string `json:"name"`
}

func named(c echo.Context) error  { _ = c.QueryParam("page"); return c.JSON(200, Place{}) }
func named2(c echo.Context) error { _ = c.QueryParam("page"); return c.JSON(200, Place{}) }
func handA(c echo.Context) error  { _ = c.QueryParam("page"); return c.JSON(200, Place{}) }
func handB(c echo.Context) error  { _ = c.QueryParam("page"); return c.JSON(200, Place{}) }

func regNamedType(g *echo.Group, h echo.HandlerFunc)          { g.GET("/namedtype", h) }
func regFuncType(g *echo.Group, h func(c echo.Context) error) { g.GET("/functype", h) }
func regClosure(g *echo.Group, h echo.HandlerFunc)            { g.GET("/closure", h) }
func register(g *echo.Group, path string, h echo.HandlerFunc) { g.GET(path, h) }
func regUnbound(g *echo.Group, h echo.HandlerFunc)            { g.GET("/unbound", h) }

func main() {
	e := echo.New()
	api := e.Group("/api")
	regNamedType(api, named)
	regFuncType(api, named2)

	closure := func(c echo.Context) error { _ = c.QueryParam("page"); return c.JSON(200, Place{}) }
	regClosure(api, closure)

	register(api, "/a", handA)
	register(api, "/b", handB)

	var hs []echo.HandlerFunc
	for _, h := range hs {
		regUnbound(api, h)
	}
	_ = e.Start(":8080")
}
