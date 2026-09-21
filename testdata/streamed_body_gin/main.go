// The streamed-body detection of issue #517, on gin.
//
// The net/http side of it shipped first, and this shape stayed broken on every
// other router for a reason worth pinning: the media type is read off the
// handler's `Content-Type` declaration, but the declaration only counts when it
// is made ON THE RESPONSE — otherwise a `Header().Set` on an OUTBOUND request
// would document a body on whatever operation reaches it. That check is
// `ResponseContext.WriterTypeRegexes`, and gin, echo and fiber had none, so the
// check could not run and nothing was claimed.
package main

import (
	"encoding/csv"
	"io"
	"os"

	"github.com/gin-gonic/gin"
)

// The framework's OWN shorthand for the declaration: c.Header(k, v).
func csvExport(c *gin.Context) {
	c.Header("Content-Type", "text/csv")
	cw := csv.NewWriter(c.Writer)
	if err := cw.Write([]string{"a", "b"}); err != nil {
		c.JSON(500, gin.H{"error": "boom"})
		return
	}
	cw.Flush()
}

// The same declaration through net/http's header map, which gin exposes as
// c.Writer.Header(). Both spellings occur in real gin code.
func download(c *gin.Context) {
	f, err := os.Open("/tmp/x.pdf")
	if err != nil {
		c.JSON(500, gin.H{"error": "boom"})
		return
	}
	defer f.Close()
	c.Writer.Header().Set("Content-Type", "application/pdf")
	_, _ = io.Copy(c.Writer, f)
}

// Negative: a csv writer over a FILE never reaches the wire, so the declaration
// beside it is not this operation's body.
func internalOnly(c *gin.Context) {
	f, _ := os.Create("/tmp/out.csv")
	defer f.Close()
	cw := csv.NewWriter(f)
	_ = cw.Write([]string{"x"})
	cw.Flush()
	c.Status(204)
}

// The framework's own raw-bytes renderer, which states the media type in an
// argument — read as the BODY by the renderer catch-all before issue #544.
func rawPDF(c *gin.Context) {
	c.Data(200, "application/pdf", []byte("%PDF"))
}

func main() {
	r := gin.Default()
	r.GET("/export.csv", csvExport)
	r.GET("/file.pdf", download)
	r.GET("/internal", internalOnly)
	r.GET("/raw.pdf", rawPDF)
	_ = r.Run(":8080")
}
