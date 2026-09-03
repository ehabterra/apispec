package aaamig

// Migrate declares a throwaway struct named like the API type, the way a schema
// migration declares only the columns it touches. The package is named so that
// it sorts BEFORE the real one: that is what decides which type a bare-name
// scan finds first.
func Migrate() any {
	// Issue see api/issue.go
	type Issue struct{ OnlyColumn string }
	return new(Issue)
}

// MigrateThing is the same trap for the version-suffixed package.
func MigrateThing() any {
	// Thing see thing/v2
	type Thing struct{ OnlyColumn string }
	return new(Thing)
}
