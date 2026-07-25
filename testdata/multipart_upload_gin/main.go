// Fixture: multipart/form-data on a framework that exposes the multipart reads
// on its own request context rather than on *http.Request (issue #207). gin's
// c.FormFile / c.MultipartForm are the same two facts as r.FormFile /
// r.ParseMultipartForm, reached through a different receiver — which is what the
// config-driven, per-framework scoping has to get right (golden rule #5).
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type UploadResult struct {
	ID string `json:"id"`
}

// uploadAvatar reads one file part by name plus a text field.
func uploadAvatar(c *gin.Context) {
	header, err := c.FormFile("avatar")
	if err != nil {
		c.JSON(http.StatusBadRequest, UploadResult{})
		return
	}
	_ = header.Filename
	_ = c.PostForm("title")
	c.JSON(http.StatusCreated, UploadResult{ID: "1"})
}

// uploadBatch names no part: c.MultipartForm() is the only evidence that the
// body is multipart, and the text field must be documented under it.
func uploadBatch(c *gin.Context) {
	form, err := c.MultipartForm()
	if err != nil {
		c.JSON(http.StatusBadRequest, UploadResult{})
		return
	}
	_ = form.File
	_ = c.PostForm("album")
	c.JSON(http.StatusCreated, UploadResult{ID: "2"})
}

func main() {
	r := gin.New()
	r.POST("/avatar", uploadAvatar)
	r.POST("/batch", uploadBatch)
	_ = r.Run(":8080")
}
