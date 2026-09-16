package hdr

// Name is a header name declared in another package, which is how a shared
// constant is normally written.
const Name = "X-From-Package"

// Settings carries a header name in a field.
type Settings struct{ Key string }

// Config is a package-level value whose field holds a name.
var Config = Settings{Key: "X-From-Field"}

// Dynamic cannot be evaluated statically.
func Dynamic() string { return "X-Unknowable" }

// FromCall sets its field from a call, so the declaration does not settle it.
var FromCall = Settings{Key: Dynamic()}

// Unset never sets Key at all, which leaves it at the zero value.
var Unset = Settings{}

// Built comes from a function, so there is no literal here to read.
var Built = build()

func build() Settings { return Settings{Key: "X-Built"} }
