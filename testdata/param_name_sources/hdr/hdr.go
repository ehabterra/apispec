package hdr

import "os"

// Name is a header name declared in another package, which is how a shared
// constant is normally written.
const Name = "X-From-Package"

// Settings carries a header name in a field.
type Settings struct{ Key string }

// Config is a package-level value whose field holds a name.
var Config = Settings{Key: "X-From-Field"}

// Dynamic is declined because it is a CALL, not because its body is
// unknowable — the body is a constant, and this is the point of the case. The
// value-resolution ladder does not follow calls: doing so means deciding which
// return a function has on the path that matters, which is a different problem
// from reading a value.
//
// Written with a constant body ON PURPOSE, so the fixture cannot be read as
// "apispec proved this was dynamic". See FromEnv for one that genuinely is.
func Dynamic() string { return "X-Unknowable" }

// FromEnv is unknowable in the stronger sense: nothing in the source decides
// it. It is declined by the same rung as Dynamic, for the same reason — both
// are calls — which is what makes the pair worth having.
func FromEnv() string { return os.Getenv("APISPEC_HEADER_NAME") }

// FromCall sets its field from a call, so the declaration does not settle it.
var FromCall = Settings{Key: Dynamic()}

// Unset never sets Key at all, which leaves it at the zero value.
var Unset = Settings{}

// Built comes from a function, so there is no literal here to read.
var Built = build()

func build() Settings { return Settings{Key: "X-Built"} }
