// Fixture: a gin group created inside a helper and returned to the caller,
// which registers on it (issue #633). The group's prefix comes from the
// Group() call inside the helper, so the caller's routes must be documented
// under it:
//
//	/ident/x          g := r.Group("/ident"); return g
//	/direct/x         return r.Group("/direct")
//	/hop/x            Outer returns Inner(r), which returns r.Group("/hop")
//	/api/nested/x     api := r.Group("/api"); return api.Group("/nested")
//	/public/v/a       Version(pub) — one helper, two callers with different
//	/admin/v/b        Version(adm)   parents; neither route leaks into the other
//	/passed/reg       the returned group passed on to a registering function
//	/witherr/x        a (group, error) helper
//
// Negative cases:
//
//	/fresh            New() returns a fresh engine that nothing mounts
//	/kept/same        Same(g) returns its parameter: the caller's prefix only
//	/pick             Pick returns one of two groups: no single prefix to choose
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func h(c *gin.Context) { c.Status(http.StatusOK) }

func Ident(r *gin.Engine) *gin.RouterGroup { g := r.Group("/ident"); return g }

func Direct(r *gin.Engine) *gin.RouterGroup { return r.Group("/direct") }

func Inner(r *gin.Engine) *gin.RouterGroup { return r.Group("/hop") }

func Outer(r *gin.Engine) *gin.RouterGroup { return Inner(r) }

func Nested(r *gin.Engine) *gin.RouterGroup {
	api := r.Group("/api")
	return api.Group("/nested")
}

func Version(parent *gin.RouterGroup) *gin.RouterGroup { return parent.Group("/v") }

func New() *gin.Engine { return gin.New() }

func Same(g *gin.RouterGroup) *gin.RouterGroup { return g }

func Passed(r *gin.Engine) *gin.RouterGroup { return r.Group("/passed") }

func register(g *gin.RouterGroup) { g.GET("/reg", h) }

func WithErr(r *gin.Engine) (*gin.RouterGroup, error) {
	g := r.Group("/witherr")
	return g, nil
}

func Pick(r *gin.Engine, alt bool) *gin.RouterGroup {
	if alt {
		return r.Group("/alt")
	}
	return r.Group("/main")
}

func main() {
	r := gin.New()

	ident := Ident(r)
	ident.GET("/x", h)

	direct := Direct(r)
	direct.GET("/x", h)

	hop := Outer(r)
	hop.GET("/x", h)

	nested := Nested(r)
	nested.GET("/x", h)

	pub := r.Group("/public")
	adm := r.Group("/admin")
	a := Version(pub)
	a.GET("/a", h)
	b := Version(adm)
	b.GET("/b", h)

	fresh := New()
	fresh.GET("/fresh", h)

	kept := r.Group("/kept")
	same := Same(kept)
	same.GET("/same", h)

	passed := Passed(r)
	register(passed)

	we, err := WithErr(r)
	if err != nil {
		return
	}
	we.GET("/x", h)

	pick := Pick(r, true)
	pick.GET("/pick", h)

	_ = r.Run(":8080")
}
