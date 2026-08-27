// Package main is a house router whose Get forwards a VARIADIC handler chain.
//
// It documents a known gap rather than a supported shape — see
// generator/testdata_variadic_wrapper_test.go.
package main

import "github.com/gin-gonic/gin"

// Reply is what the handlers return.
type Reply struct {
	Name string `json:"name"`
}

// Router wraps gin, forwarding whatever chain the caller supplies.
type Router struct{ engine *gin.Engine }

// Get takes the chain variadically, exactly as gin's own GET does.
func (r *Router) Get(path string, handlers ...gin.HandlerFunc) {
	r.engine.GET(path, handlers...)
}

func auth(c *gin.Context)     { c.Next() }
func endpoint(c *gin.Context) { c.JSON(200, Reply{Name: "endpoint"}) }
func plain(c *gin.Context)    { c.JSON(200, Reply{Name: "plain"}) }

func main() {
	r := &Router{engine: gin.New()}
	r.Get("/users", auth, endpoint) // middleware, then the endpoint handler
	r.Get("/health", plain)         // handler only
	_ = r.engine.Run(":8080")
}
