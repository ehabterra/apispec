// Package main is a house router whose Get forwards a VARIADIC handler chain —
// the wiring style where the endpoint handler is LAST and everything before it
// is middleware (issue #416).
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

// auth guards the route and is NOT its endpoint. Its doc comment is here so the
// summary can tell the two apart: attributing the operation to the middleware
// took this sentence as the endpoint's description.
func auth(c *gin.Context) { c.Next() }

// endpoint serves the users route.
func endpoint(c *gin.Context) { c.JSON(200, Reply{Name: "endpoint"}) }

// plain serves the health route, with no middleware ahead of it.
func plain(c *gin.Context) { c.JSON(200, Reply{Name: "plain"}) }

func main() {
	r := &Router{engine: gin.New()}
	r.Get("/users", auth, endpoint) // middleware, then the endpoint handler
	r.Get("/health", plain)         // handler only
	_ = r.engine.Run(":8080")
}
