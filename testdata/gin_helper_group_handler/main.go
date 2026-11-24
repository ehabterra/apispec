// A helper that receives both the router group and the handler (issue #615):
// each route must be documented once, under the group's prefix, with the
// handler the call site passed.
package main

import "github.com/gin-gonic/gin"

// Place is the body every handler in this fixture returns.
type Place struct {
	Name string `json:"name"`
}

func named(c *gin.Context)  { _ = c.Query("page"); c.JSON(200, Place{}) }
func named2(c *gin.Context) { _ = c.Query("page"); c.JSON(200, Place{}) }
func handA(c *gin.Context)  { _ = c.Query("page"); c.JSON(200, Place{}) }
func handB(c *gin.Context)  { _ = c.Query("page"); c.JSON(200, Place{}) }

// The helper receives both the group and the handler.
func regNamedType(r *gin.RouterGroup, h gin.HandlerFunc)   { r.GET("/namedtype", h) }
func regFuncType(r *gin.RouterGroup, h func(*gin.Context)) { r.GET("/functype", h) }
func regClosure(r *gin.RouterGroup, h gin.HandlerFunc)     { r.GET("/closure", h) }

// The helper receives the path too; two call sites must stay two routes.
func register(r *gin.RouterGroup, path string, h gin.HandlerFunc) { r.GET(path, h) }

// The handler parameter is never bound to a concrete function: it must stay
// unresolved rather than borrow a handler from another call site.
func regUnbound(r *gin.RouterGroup, h gin.HandlerFunc) { r.GET("/unbound", h) }

func plain(c *gin.Context) { c.JSON(200, Place{}) }

// A helper building a sub-group from its parameter, called twice on the same
// group (issue #637): each call site is its own frame, so its suffix resolves.
func versionReg(parent *gin.RouterGroup, suffix string) {
	g := parent.Group(suffix)
	g.GET("/in", plain)
}

// A helper forwarding its group to another helper, fed two different groups
// (issue #636): left as it is today, see the structural test.
func idReg(g *gin.RouterGroup, p string)        { g.GET(p, plain) }
func wrapReg(parent *gin.RouterGroup, p string) { idReg(parent, p) }

func main() {
	r := gin.New()
	api := r.Group("/api")
	regNamedType(api, named)
	regFuncType(api, named2)

	closure := func(c *gin.Context) { _ = c.Query("page"); c.JSON(200, Place{}) }
	regClosure(api, closure)

	register(api, "/a", handA)
	register(api, "/b", handB)

	var hs []gin.HandlerFunc
	for _, h := range hs {
		regUnbound(api, h)
	}
	vr := r.Group("/vr")
	versionReg(vr, "/c")
	versionReg(vr, "/d")

	ra := r.Group("/ra")
	wrapReg(ra, "/one")
	rb := r.Group("/rb")
	wrapReg(rb, "/two")

	_ = r.Run()
}
