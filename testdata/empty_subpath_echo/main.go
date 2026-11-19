// An empty registration path under a group is the group's own path (issue
// #609): echo serves `articles.GET("", h)` at /api/articles.
package main

import (
	"net/http"

	"github.com/labstack/echo/v4"
)

type Article struct {
	Slug string `json:"slug"`
}

func listArticles(c echo.Context) error  { return c.JSON(http.StatusOK, []Article{}) }
func createArticle(c echo.Context) error { return c.JSON(http.StatusCreated, Article{}) }
func getArticle(c echo.Context) error    { return c.JSON(http.StatusOK, Article{}) }

func main() {
	e := echo.New()
	api := e.Group("/api")
	articles := api.Group("/articles")
	articles.GET("", listArticles)
	articles.POST("", createArticle)
	articles.GET("/:slug", getArticle)
	e.Logger.Fatal(e.Start(":8080"))
}
