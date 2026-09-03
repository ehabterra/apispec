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
