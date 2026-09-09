package main

import "github.com/gin-gonic/gin"

type Item struct {
	ID   int    `json:"id" xml:"id"`
	Name string `json:"name" xml:"name"`
}

func getItemJSON(c *gin.Context) { c.JSON(200, Item{}) }

func getItemXML(c *gin.Context) { c.XML(200, Item{}) }

func getItemYAML(c *gin.Context) { c.YAML(200, Item{}) }

func plain(c *gin.Context) { c.String(200, "ok") }

func page(c *gin.Context) { c.HTML(200, "index.tmpl", nil) }

func main() {
	r := gin.Default()
	r.GET("/items/json", getItemJSON)
	r.GET("/items/xml", getItemXML)
	r.GET("/items/yaml", getItemYAML)
	r.GET("/plain", plain)
	r.GET("/page", page)
	_ = r.Run(":8080")
}
