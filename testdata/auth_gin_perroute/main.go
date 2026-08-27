// Package main demonstrates gin per-route middleware: r.GET(path, mw, handler)
// where the middleware precedes the final handler arg.
package main

import (
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

// User is the resource the protected handlers return.
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// CreateUserRequest is the body createUser binds.
type CreateUserRequest struct {
	Name string `json:"name"`
}

// jwtAuth returns a gin middleware whose closure validates a JWT.
func jwtAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		_, _ = jwt.Parse(c.GetHeader("Authorization"), func(t *jwt.Token) (interface{}, error) {
			return nil, nil
		})
		c.Next()
	}
}

// rateLimit is a second per-route middleware, so the registration carries a
// chain rather than a single guard: only reading the LAST argument attributes
// the operation correctly.
func rateLimit() gin.HandlerFunc {
	return func(c *gin.Context) { c.Next() }
}

// getUser returns one user by id.
func getUser(c *gin.Context) {
	c.JSON(200, User{ID: c.Param("id"), Name: "ada"})
}

// createUser stores a new user.
func createUser(c *gin.Context) {
	var req CreateUserRequest
	_ = c.ShouldBindJSON(&req)
	c.JSON(201, User{ID: "1", Name: req.Name})
}

func health(c *gin.Context) { c.String(200, "ok") }

func main() {
	r := gin.New()
	r.GET("/users/:id", jwtAuth(), getUser)              // one middleware
	r.POST("/users", jwtAuth(), rateLimit(), createUser) // a chain of two
	r.GET("/health", health)                             // open
	_ = r.Run(":8080")
}
