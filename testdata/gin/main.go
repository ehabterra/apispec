package main

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// User represents a user model.
type User struct {
	ID   int    `json:"id"`
	Name string `json:"name" binding:"required"`
}

var users = []User{
	{ID: 1, Name: "Alice"},
	{ID: 2, Name: "Bob"},
}

// Get all users
func ListUsers(c *gin.Context) {
	c.JSON(http.StatusOK, users)
}

// Get a user by ID
func GetUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	for _, u := range users {
		if u.ID == id {
			c.JSON(http.StatusOK, u)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
}

// Create a new user
func CreateUser(c *gin.Context) {
	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	user.ID = len(users) + 1
	users = append(users, user)
	c.JSON(http.StatusCreated, user)
}

// Update an existing user
func UpdateUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var user User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	for i, u := range users {
		if u.ID == id {
			user.ID = id
			users[i] = user
			c.JSON(http.StatusOK, user)
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
}

// Delete a user
func DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	for i, u := range users {
		if u.ID == id {
			users = append(users[:i], users[i+1:]...)
			c.JSON(http.StatusNoContent, gin.H{})
			return
		}
	}
	c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
}

func main() {
	r := gin.Default()
	g := r.Group("/users")
	g.GET("/", ListUsers)
	g.GET("/:id", GetUser)
	UserRoutes(g)
	r.Run(":8080")
}

func UserRoutes(rg *gin.RouterGroup) {
	rg.POST("/", CreateUser)
	rg.PUT("/:id", UpdateUser)
	rg.DELETE("/:id", DeleteUser)

}
