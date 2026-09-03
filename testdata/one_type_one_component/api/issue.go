package api

// Issue is the real API type.
type Issue struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
	State string `json:"state"`
}
