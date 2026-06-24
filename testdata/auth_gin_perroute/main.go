// Package main demonstrates gin per-route middleware: r.GET(path, mw, handler)
// where the middleware precedes the final handler arg.
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// jwtAuth returns a gin middleware whose closure validates a JWT.
func jwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, _ = jwt.Parse(c.GetHeader("Authorization"), func(t *jwt.Token) (interface{}, error) {
			return nil, nil
		})
		c.Next()
	}
}

func getUser(c *gin.Context) { c.JSON(200, gin.H{}) }
func health(c *gin.Context)  { c.String(200, "ok") }

func main() {
	r := gin.New()
	r.GET("/users/:id", jwtAuth(), getUser) // protected (per-route mw)
	r.GET("/health", health)                // open
	_ = r.Run(":8080")
}
