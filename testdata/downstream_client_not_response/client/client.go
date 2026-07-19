// Package client is a downstream HTTP client. Its json.Marshal serializes the
// OUTBOUND request — the client method has no ResponseWriter, so the request
// type must NOT surface as the outer route's response (issue #195).
package client

import (
	"encoding/json"
	"net/http"
)

type FetchRequest struct {
	Amount   int               `json:"amount"`
	Metadata map[string]string `json:"metadata"`
}

type FetchResponse struct {
	Key string `json:"key"`
}

type Client struct{}

func (c *Client) Fetch(params *FetchRequest) (*FetchResponse, error) {
	payload, err := json.Marshal(params) // outbound request marshal — goes to the wire, not w
	if err != nil {
		return nil, err
	}
	_, _ = http.Post("http://svc/fetch", "application/json", nil)
	_ = payload
	var resp FetchResponse
	_ = json.Unmarshal([]byte(`{}`), &resp)
	return &resp, nil
}
