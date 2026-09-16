// Package setting mirrors the shape a project's configuration has: package
// level vars whose values are read at RUNTIME, not constants.
package setting

// AuthUser is what a reverse-proxy auth header is called, and it is not
// knowable from the source — the deployment decides it.
var AuthUser = key("REVERSE_PROXY_AUTHENTICATION_USER").MustString("X-WEBAUTH-USER")

// Fixed is a var that really does hold a literal, which must keep resolving.
var Fixed = "X-Fixed"

// Raw is a RAW string literal. Its rendering keeps the backticks it was written
// with, and a header called `X-Raw` — backticks included — is not one any client
// can send.
var Raw = `X-Raw`

// Alias holds another NAME, and its rendering is that name — a string
// indistinguishable from a literal spelling the same thing. Only the recorded
// KIND separates them, which is why metadata has to carry it: without it this
// documents a Go identifier as a header.
var Alias = Fixed

type k string

func key(s string) k { return k(s) }

func (s k) MustString(def string) string { return def }
