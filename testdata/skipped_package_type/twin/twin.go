package twin

// Row shares the NAME and nothing else. It is a different type, in a package
// the route never mentions.
type Row struct {
	Sheet string `json:"sheet"`
}
