// The chi twin of this fixture proves the rule; this one proves it is not chi's.
//
// Wrapper derivation and the request-source check are shared across frameworks,
// and the only per-framework part is the configured `requestContext` — so the
// house context here holds a *gin.Context rather than an *http.Request, and the
// request is two accessors from the root instead of one (issue #513, golden
// rule #5).
package main

import (
	"encoding/json"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Settings struct {
	Name string `json:"name"`
}

// providerReply belongs to a third-party API's client and is never a request
// body of this service.
type providerReply struct {
	Contacts []string `json:"contacts"`
}

// Ctx is the project's own context, holding gin's.
type Ctx struct {
	G *gin.Context
}

// Bind reads THIS request's body, reached as c.G.Request.Body.
func (c *Ctx) Bind(dst any) error {
	return json.NewDecoder(c.G.Request.Body).Decode(dst)
}

type client struct{}

// fetch has Bind's shape exactly and reads an *http.Response instead.
func (c *client) fetch(out any) error {
	resp, err := http.Get("https://provider.example/contacts")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

var remote = &client{}

// createSettings decodes its body through the house wrapper.
func createSettings(g *gin.Context) {
	c := &Ctx{G: g}
	var in Settings
	if err := c.Bind(&in); err != nil {
		g.Status(http.StatusBadRequest)
		return
	}
	g.JSON(http.StatusCreated, in)
}

// syncContacts reads no body of its own.
func syncContacts(g *gin.Context) {
	var out providerReply
	if err := remote.fetch(&out); err != nil {
		g.Status(http.StatusBadGateway)
		return
	}
	g.Status(http.StatusNoContent)
}

// updateSettings does both: Settings is the answer.
func updateSettings(g *gin.Context) {
	c := &Ctx{G: g}
	var in Settings
	if err := c.Bind(&in); err != nil {
		g.Status(http.StatusBadRequest)
		return
	}
	var out providerReply
	if err := remote.fetch(&out); err != nil {
		g.Status(http.StatusBadGateway)
		return
	}
	g.JSON(http.StatusOK, in)
}

func main() {
	r := gin.Default()
	r.POST("/settings", createSettings)
	r.POST("/contacts/sync", syncContacts)
	r.PUT("/settings", updateSettings)
	_ = r.Run(":8080")
}
