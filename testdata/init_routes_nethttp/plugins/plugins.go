// Package plugins registers its routes from init(), the plugin-registration
// shape: main only blank-imports it.
package plugins

import (
	"encoding/json"
	"net/http"
)

type Plugin struct {
	Name string `json:"name"`
}

func list(w http.ResponseWriter, _ *http.Request) {
	_ = json.NewEncoder(w).Encode([]Plugin{})
}

func init() {
	http.HandleFunc("/plugins", list)
}
