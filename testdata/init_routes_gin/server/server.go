// Package server holds the router every package registers onto.
package server

import "github.com/gin-gonic/gin"

// Engine is created at package level; API is a group on it, set up in init.
var (
	Engine = gin.New()
	API    *gin.RouterGroup
)

func init() {
	API = Engine.Group("/api")
}
