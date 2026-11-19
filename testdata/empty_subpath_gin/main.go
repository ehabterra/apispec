// An empty registration path under a group is the group's own path (issue
// #609): gin serves `users.GET("", h)` at /users, not /users/. An explicit
// "/" is a real trailing slash and must stay one.
package main

import "github.com/gin-gonic/gin"

type User struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type Status struct {
	OK bool `json:"ok"`
}

func listUsers(c *gin.Context)  { c.JSON(200, []User{}) }
func createUser(c *gin.Context) { c.JSON(201, User{}) }
func getUser(c *gin.Context)    { c.JSON(200, User{}) }
func listOrgs(c *gin.Context)   { c.JSON(200, []string{}) }
func health(c *gin.Context)     { c.JSON(200, Status{}) }
func root(c *gin.Context)       { c.JSON(200, Status{}) }

func main() {
	r := gin.New()
	r.GET("", root) // empty path on the engine itself: /

	api := r.Group("/api")

	users := api.Group("/users")
	users.GET("", listUsers)
	users.POST("", createUser)
	users.GET("/:id", getUser)

	// A group whose own path is empty adds nothing to the prefix.
	v := api.Group("")
	v.GET("/health", health)

	// Explicit trailing slash: gin registers /api/orgs/.
	orgs := api.Group("/orgs")
	orgs.GET("/", listOrgs)

	r.Run()
}
