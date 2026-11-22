// A handler closure held in a local variable is the handler (issue #610).
//
//	handler := func(c *gin.Context) { … }
//	router.GET("/geo", handler)
//
// The route used to be found with nothing under it — no parameters, no
// responses — while the same literal written inline was fully documented.
package main

import (
	"os"

	"github.com/gin-gonic/gin"
)

type Geo struct {
	Lat float64 `json:"lat"`
	Lng float64 `json:"lng"`
}

type Place struct {
	Name string `json:"name"`
}

type Other struct {
	Code int `json:"code"`
}

// Assigned once, registered at two routes.
func registerGeo(router *gin.RouterGroup) {
	handler := func(c *gin.Context) {
		_ = c.Query("count")
		c.JSON(200, Geo{})
	}
	router.GET("/geo", handler)
	router.GET("/geo/:format", handler)
}

// Assigned and registered once.
func registerOnce(router *gin.RouterGroup) {
	h := func(c *gin.Context) {
		_ = c.Query("q")
		c.JSON(200, Place{})
	}
	router.GET("/once", h)
}

// The control: the identical shape written inline.
func registerInline(router *gin.RouterGroup) {
	router.GET("/inline", func(c *gin.Context) {
		_ = c.Query("count")
		c.JSON(200, Geo{})
	})
}

// A factory's closure reads the factory's PARAMETERS. The binding guard the fix
// adds (a call's parameters bind only inside the frame it enters) has to keep
// counting the closure as part of that frame.
func respond(status int, v any) gin.HandlerFunc {
	return func(c *gin.Context) { c.JSON(status, v) }
}

func bind(dst any) gin.HandlerFunc {
	return func(c *gin.Context) {
		_ = c.ShouldBindJSON(dst)
		c.JSON(201, Place{})
	}
}

type NewPlace struct {
	Name string `json:"name"`
}

// Two different closures assigned to one variable: which one serves the route
// depends on runtime state, so the route must NOT be attributed to either.
func registerAmbiguous(router *gin.RouterGroup) {
	amb := func(c *gin.Context) { c.JSON(200, Place{}) }
	if os.Getenv("ALT") != "" {
		amb = func(c *gin.Context) { c.JSON(200, Other{}) }
	}
	router.GET("/ambiguous", amb)
}

func main() {
	r := gin.New()
	api := r.Group("/api")
	registerGeo(api)
	registerOnce(api)
	registerInline(api)
	registerAmbiguous(api)
	api.GET("/factory/geo", respond(200, Geo{}))
	api.POST("/factory/place", bind(&NewPlace{}))
	r.Run()
}
