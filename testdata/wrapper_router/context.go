// The other half of a house router: the project's own context. Handlers answer
// through it, so the response body, the request body and the parameters are all
// read through methods of this type rather than through the framework's.
//
// The shape mirrors gitea's services/context.Base: a status set with WriteHeader
// and a body encoded to the same writer, both taking the caller's values as
// parameters.
package main

import (
	"encoding/json"
	"net/http"
)

type Ctx struct {
	Resp http.ResponseWriter
	Req  *http.Request
}

// JSON answers with a status and a body — two roles reached through two
// different calls in one method.
func (c *Ctx) JSON(status int, content any) {
	c.Resp.Header().Set("Content-Type", "application/json")
	c.Resp.WriteHeader(status)
	_ = json.NewEncoder(c.Resp).Encode(content)
}

// Bind reads the request body into dst.
func (c *Ctx) Bind(dst any) error {
	return json.NewDecoder(c.Req.Body).Decode(dst)
}

// Query reads a query parameter by name.
func (c *Ctx) Query(name string) string {
	return c.Req.URL.Query().Get(name)
}
