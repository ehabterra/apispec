// Fixture: a gin group built by a factory and used by the caller, where the
// factory's local is named like the caller's variable (issue #628). Gin has no
// Mount, so the prefix comes from the group the factory returns:
//
//	/v1     v1 := V1(r)      factory local `v1`
//	/v2     v2 := V2(r)      factory local `g` (control)
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

func h(c *gin.Context) { c.Status(http.StatusOK) }

func V1(r *gin.Engine) *gin.RouterGroup { v1 := r.Group("/v1"); return v1 }
func V2(r *gin.Engine) *gin.RouterGroup { g := r.Group("/v2"); return g }

func main() {
	r := gin.New()

	v1 := V1(r)
	v1.GET("/same", h)

	v2 := V2(r)
	v2.GET("/other", h)

	_ = r.Run(":8080")
}
