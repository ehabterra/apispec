// Package httpx holds the one shared decode helper. It is GENERIC: the
// decoder.Decode(&data) inside it is a SINGLE call site shared by every route,
// and only the type argument tells one route's body from another's.
package httpx

import (
	"encoding/json"
	"net/http"
)

func DecodeJSON[TData any](r *http.Request) (TData, error) {
	var data TData
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&data); err != nil {
		return data, err
	}
	return data, nil
}
