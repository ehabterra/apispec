// Fixture: a header read is a request parameter only when it reads the REQUEST's
// headers. The same net/http.Header type is on both sides of an exchange, so
// `Get` on it says nothing by itself — the receiver's origin does.
//
// A response header a handler SETS or reads back (`c.Response().Header()`,
// `w.Header()`) is something the server sends; documenting it as a parameter
// tells clients to send a header the API never reads. Seen on a real echo
// project as a spurious `in: header` parameter.
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/labstack/echo/v4"
)

type Item struct {
	ID string `json:"id"`
}

// echoHandler reads one header from each side of the exchange.
func echoHandler(c echo.Context) error {
	// REQUEST header: a client sends this -> a parameter.
	_ = c.Request().Header.Get("X-Tenant")

	// RESPONSE header: the server's own, read back after middleware set it.
	// Not a parameter.
	_ = c.Response().Header().Get("X-Trace-Id")

	return c.JSON(http.StatusOK, Item{})
}

// plainHandler is the same split in net/http terms.
func plainHandler(w http.ResponseWriter, r *http.Request) {
	// REQUEST header -> a parameter.
	_ = r.Header.Get("X-Api-Key")

	// RESPONSE header -> not a parameter.
	_ = w.Header().Get("Content-Type")

	// The same two sides once more, through a variable rather than a chain: the
	// receiver's origin is then an assignment, not a chain parent.
	rh := r.Header
	_ = rh.Get("X-Signature") // request -> a parameter

	wh := w.Header()
	_ = wh.Get("X-Cache") // response -> not a parameter

	w.WriteHeader(http.StatusNoContent)
}

// ginHandler is the same split again: gin's writer is its own type, so the
// exclusion cannot be written for one framework's writer alone.
func ginHandler(c *gin.Context) {
	// REQUEST header -> a parameter.
	_ = c.Request.Header.Get("X-Gin-Tenant")

	// RESPONSE header -> not a parameter.
	_ = c.Writer.Header().Get("X-Gin-Trace")

	c.JSON(http.StatusOK, Item{})
}

func main() {
	e := echo.New()
	e.GET("/items", echoHandler)

	g := gin.New()
	g.GET("/gin-items", ginHandler)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /plain", plainHandler)

	_ = e.Start(":8080")
	_ = http.ListenAndServe(":8081", mux)
}
