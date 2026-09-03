// Package thing sits in a directory named v2, the way a module major version is
// laid out: the DECLARED package name is "thing" while the path's last segment
// is "v2". Reading the name off the path answers "v2", so a `thing.`-qualified
// type could never be resolved and fell back to a bare-name scan — which found
// aaamig's throwaway struct instead (issue #457).
package thing

// Thing is the real type behind a version-suffixed path.
type Thing struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
