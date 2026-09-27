// Package main is the gin shape of issue #569: the request's headers reach a
// gin handler through c.Request, and an outbound response's through the
// http.Client call that returned it. Only the first are parameters.
//
// Documented:     X-Request-Id (c.Request.Header.Get), X-Trace (a helper
//                 handed c.Request.Header)
// Not documented: X-Rate-Remaining, Retry-After (off an http.Get response)
package main

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Item struct {
	ID string `json:"id"`
}

func backoff(h http.Header) string { return h.Get("Retry-After") }

func traceOf(h http.Header) string { return h.Get("X-Trace") }

func list(c *gin.Context) {
	id := c.Request.Header.Get("X-Request-Id")
	_ = traceOf(c.Request.Header)

	if resp, err := http.Get("https://upstream.example.test/v1"); err == nil {
		_ = resp.Header.Get("X-Rate-Remaining")
		_ = backoff(resp.Header)
		_ = resp.Body.Close()
	}
	c.JSON(http.StatusOK, []Item{{ID: id}})
}

func main() {
	r := gin.New()
	r.GET("/items", list)
	_ = r.Run(":8080")
}
