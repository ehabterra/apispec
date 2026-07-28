// Copyright 2026 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package generator

import (
	"testing"
)

// TestTestdata_ResponseHeaderParam pins that a header read documents a request
// parameter only when it reads the REQUEST's headers.
//
// `net/http.Header` is the header map of both sides of an exchange, so the
// receiver type says nothing: only where the header came from does. A header the
// server SENDS, read back through the response writer, became an `in: header`
// parameter — telling clients to send something the API never looks at. Seen on a
// real echo project as a spurious parameter from
// `c.Response().Header().Get(...)`.
//
// Each framework's writer is a different type and the header pattern is shared
// across all of them, so all three are checked here — a fix written for one
// framework's writer leaves the others emitting the bogus parameter.
func TestTestdata_ResponseHeaderParam(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "response_header_param", nil)
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	cases := []struct {
		path    string
		want    []string // request headers, which a client really does send
		notWant []string // response headers, which it must not be asked to send
	}{
		// echo: c.Request().Header.Get vs c.Response().Header().Get
		{path: "/items", want: []string{"X-Tenant"}, notWant: []string{"X-Trace-Id"}},
		// gin: c.Request.Header.Get vs c.Writer.Header().Get
		{path: "/gin-items", want: []string{"X-Gin-Tenant"}, notWant: []string{"X-Gin-Trace"}},
		// net/http, both as a chain and through a variable receiver, whose origin
		// is an assignment rather than a chain parent.
		{
			path:    "/plain",
			want:    []string{"X-Api-Key", "X-Signature"},
			notWant: []string{"Content-Type", "X-Cache"},
		},
	}

	for _, tc := range cases {
		op := opFor(out.Paths[tc.path], "GET")
		if op == nil {
			t.Errorf("GET %s missing; have %v", tc.path, mapPathKeys(out.Paths))
			continue
		}
		got := map[string]string{}
		for _, p := range op.Parameters {
			got[p.Name] = p.In
		}
		for _, name := range tc.want {
			if got[name] != "header" {
				t.Errorf("GET %s: request header %q is in=%q, want header; have %v", tc.path, name, got[name], got)
			}
		}
		for _, name := range tc.notWant {
			if _, ok := got[name]; ok {
				t.Errorf("GET %s: %q is a RESPONSE header the server sends, not a parameter; have %v", tc.path, name, got)
			}
		}
	}
}
