package mod

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// User is the response body of the module's handlers.
type User struct {
	ID int `json:"id"`
}

// RegisterRouter is the per-feature registrar most gin projects call once per
// module, with the group created inline in the argument.
func RegisterRouter(r *gin.RouterGroup) {
	r.POST("/login", LoginHandler)
	r.GET("/profile", ProfileHandler)
}

func LoginHandler(c *gin.Context)   { c.JSON(http.StatusOK, User{ID: 1}) }
func ProfileHandler(c *gin.Context) { c.JSON(http.StatusOK, User{ID: 2}) }
