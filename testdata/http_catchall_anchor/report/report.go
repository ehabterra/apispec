package report

import "context"

// Payload is INTERNAL analytics data. It is never written to a response.
type Payload struct {
	Rows int `json:"rows"`
}

// Data is named like a renderer and takes a payload as its second argument,
// which is exactly the shape the net/http catch-all matches on. It receives no
// http.ResponseWriter, so it cannot be writing this endpoint's response.
func Data(ctx context.Context, p Payload) error {
	_ = ctx
	_ = p
	return nil
}
