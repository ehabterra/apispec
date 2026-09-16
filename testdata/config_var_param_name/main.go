// Package main reads request headers whose names come from configuration.
//
// A parameter's name is what the CLIENT sends, so it has to be a value the
// source settles. A package-level var initialised by a call does not settle
// one, and rendering its initializer produced a header name no client could
// ever send (issue #452).
package main

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/ehabterra/apispec/testdata/config_var_param_name/setting"
)

// Reply is what the handler returns.
type Reply struct {
	OK bool `json:"ok"`
}

// userName reads the header the deployment configured, one call deeper than the
// handler — the shape the real project has.
func userName(req *http.Request) string {
	return strings.TrimSpace(req.Header.Get(setting.AuthUser))
}

func whoami(w http.ResponseWriter, r *http.Request) {
	_ = userName(r)
	// Alongside it, two names the source DOES settle, which must survive.
	_ = r.Header.Get(setting.Fixed)
	_ = r.Header.Get(setting.Raw)
	_ = r.Header.Get(setting.Alias)
	_ = r.Header.Get("X-Literal")
	_ = json.NewEncoder(w).Encode(Reply{OK: true})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /whoami", whoami)
	_ = http.ListenAndServe(":8080", mux)
}
