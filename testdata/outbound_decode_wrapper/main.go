// A decoder wrapper and an outbound HTTP client have the same shape: a method
// that forwards its own parameter into a json decode. Only where the bytes come
// from tells them apart, and wrapper derivation did not look — so the client's
// method was derived as a request-body pattern, and every handler that called it
// documented the provider's reply as its own request body (issue #513).
//
// The four shapes sit side by side because three of them must keep working: the
// bug is not "derive less", it is "derive from the request".
package main

import (
	"encoding/json"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"
)

type Settings struct {
	Name string `json:"name"`
}

type Item struct {
	SKU string `json:"sku"`
}

// providerReply belongs to a third-party API's client. It is not a request body
// of this service, and must never be documented as one.
type providerReply struct {
	Contacts []string `json:"contacts"`
}

// Ctx is the project's own context, holding the request in a FIELD — the shape a
// house context has, and the reason the source check cannot read the root of
// `c.Req.Body` and stop there.
type Ctx struct {
	Resp http.ResponseWriter
	Req  *http.Request
}

// Bind is a genuine decoder wrapper: it reads THIS request's body.
func (c *Ctx) Bind(dst any) error {
	return json.NewDecoder(c.Req.Body).Decode(dst)
}

// client calls a third-party API.
type client struct{ base string }

// fetch has Bind's shape exactly — a method forwarding its parameter into a
// decode — and reads an *http.Response instead. Nothing may be derived from it.
func (c *client) fetch(out any) error {
	resp, err := http.Get(c.base + "/contacts")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// readFrom decodes whatever it is handed. Whether that is a request body is
// knowable only at the call site, so the derived pattern has to carry the check
// rather than answer it here.
func (c *Ctx) readFrom(src io.Reader, dst any) error {
	return json.NewDecoder(src).Decode(dst)
}

// readBody reads the request into bytes first, then unmarshals them — the Go
// habit of reaching for io.ReadAll, and a decode one hop further from the
// request than a decoder chain.
func (c *Ctx) readBody(dst any) error {
	data, err := io.ReadAll(c.Req.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

// fetchSentRequest decodes the request that was SENT. An *http.Response carries
// the outbound *http.Request in a field, so this reaches a request-typed value
// one accessor in — exactly like a house context's `c.Req.Body`, and not the
// request being served.
func (c *client) fetchSentRequest(out any) error {
	resp, err := http.Get(c.base + "/echo")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Request.Body).Decode(out)
}

// fetchBytes is readBody's outbound twin, reading the provider's reply the same
// way. The two differ only in which reader they are given.
func (c *client) fetchBytes(out any) error {
	resp, err := http.Get(c.base + "/items")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, out)
}

type handler struct {
	svc *client
}

// createSettings decodes its body through the house wrapper: a request body.
func (h *handler) createSettings(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{Resp: w, Req: r}
	var in Settings
	if err := c.Bind(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// syncContacts decodes ONLY the provider's reply, and reads no body of its own.
// It must document no request body at all.
func (h *handler) syncContacts(w http.ResponseWriter, r *http.Request) {
	var out providerReply
	if err := h.svc.fetch(&out); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// updateSettings does both, which is how the bug was found in the wild: the
// handler plainly decodes Settings, and the outbound decode happens four calls
// down. Settings is the answer; the provider's type must not replace it.
func (h *handler) updateSettings(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{Resp: w, Req: r}
	var in Settings
	if err := c.Bind(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var out providerReply
	if err := h.svc.fetch(&out); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// createItem hands the wrapper the request's own body: a request body.
func (h *handler) createItem(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{Resp: w, Req: r}
	var in Item
	if err := c.readFrom(r.Body, &in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

// importItem hands the same wrapper a file. Same method, same derived pattern,
// and this call site is not a request body — which is what the carried check is
// for.
func (h *handler) importItem(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{Resp: w, Req: r}
	f, err := openSeed()
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	defer f.Close()
	var in Item
	if err := c.readFrom(f, &in); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func openSeed() (io.ReadCloser, error) { return http.NoBody, nil }

// patchSettings reads its body through io.ReadAll: a request body, and one the
// plain (unwrapped) handler shape misses just as readily.
func (h *handler) patchSettings(w http.ResponseWriter, r *http.Request) {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	var in Settings
	if err := json.Unmarshal(data, &in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// replaceSettings reads its body the same way, through the house wrapper.
func (h *handler) replaceSettings(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{Resp: w, Req: r}
	var in Settings
	if err := c.readBody(&in); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// echoRemote reads only the outbound request's body, and must document none.
func (h *handler) echoRemote(w http.ResponseWriter, r *http.Request) {
	var out providerReply
	if err := h.svc.fetchSentRequest(&out); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// listRemoteItems reads only the provider's bytes, and must document no body.
func (h *handler) listRemoteItems(w http.ResponseWriter, r *http.Request) {
	var out providerReply
	if err := h.svc.fetchBytes(&out); err != nil {
		w.WriteHeader(http.StatusBadGateway)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func main() {
	h := &handler{svc: &client{base: "https://provider.example"}}

	r := chi.NewRouter()
	r.Post("/settings", h.createSettings)
	r.Put("/settings", h.updateSettings)
	r.Post("/contacts/sync", h.syncContacts)
	r.Post("/items", h.createItem)
	r.Post("/items/import", h.importItem)
	r.Patch("/settings", h.patchSettings)
	r.Post("/settings/replace", h.replaceSettings)
	r.Post("/items/remote", h.listRemoteItems)
	r.Post("/items/echo", h.echoRemote)

	_ = http.ListenAndServe(":8080", r)
}
