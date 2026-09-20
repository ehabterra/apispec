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
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_OutboundDecodeWrapper pins which decoder methods a project's own
// types may contribute a request body from (issue #513).
//
// Wrapper derivation recognises a decoder by SHAPE — a method forwarding its own
// parameter into a json decode — and an outbound HTTP client has that shape too.
// Derivation did not consult the source check that extraction uses, so
// `func (c *client) fetch(out any)` reading an *http.Response was derived as
// readily as `func (c *Ctx) Bind(dst any)` reading c.Req.Body, and the derived
// pattern then documented the provider's type on every handler that called it.
//
// Run with chi's defaults and nothing else: the whole point is what a user gets
// by pointing apispec at the project.
func TestTestdata_OutboundDecodeWrapper(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "outbound_decode_wrapper", intspec.DefaultChiConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	cases := []struct {
		name         string
		method, path string
		// want is the component the request body must name, or "" for no
		// request body at all.
		want string
		why  string
	}{
		{
			name: "house wrapper over the request",
			// c.Bind(&in) reads c.Req.Body — the request is held in a FIELD, so
			// the source check has to walk past the root of the chain to find
			// it. Reading the root alone would refuse this and derive nothing.
			method: "POST", path: "/settings", want: "Settings",
			why: "the house context's Bind reads this request's body",
		},
		{
			name: "outbound client with the same shape",
			// The only decode this handler reaches is the provider's reply.
			method: "POST", path: "/contacts/sync", want: "",
			why: "the only decode reachable is an *http.Response, which is never a request body",
		},
		{
			name: "both, in one handler",
			// The shape the bug was found in: the handler decodes its own body
			// and calls the client afterwards. Both candidates are concrete, so
			// the later one used to win.
			method: "PUT", path: "/settings", want: "Settings",
			why: "the handler's own body must not be displaced by an outbound decode",
		},
		{
			name: "source carried from the caller, given the request",
			// readFrom decodes whatever it is handed, so the derived pattern
			// carries the check to the call site instead of answering it.
			method: "POST", path: "/items", want: "Item",
			why: "the wrapper is handed r.Body here",
		},
		{
			name: "source carried from the caller, given a file",
			// Same method, same derived pattern, different argument.
			method: "POST", path: "/items/import", want: "",
			why: "the same wrapper is handed a file here, which is not a request body",
		},
		{
			name: "read into bytes first, in a plain handler",
			// `data, _ := io.ReadAll(r.Body)` then `json.Unmarshal(data, &v)`.
			// This one was wrong on its own, with no wrapper involved: the
			// source check could not see past the read, so it answered "not the
			// request" and the handler documented no body at all.
			method: "PATCH", path: "/settings", want: "Settings",
			why: "bytes read from the request body are still the request body",
		},
		{
			name:   "read into bytes first, through the wrapper",
			method: "POST", path: "/settings/replace", want: "Settings",
			why: "the same read, one call further out",
		},
		{
			name: "read into bytes first, from the provider",
			// The exact shape above, given the other reader. Naming the readers
			// must not turn every ReadAll into a request body.
			method: "POST", path: "/items/remote", want: "",
			why: "an *http.Response read the same way is still not a request body",
		},
		{
			name: "the request a response carries",
			// An *http.Response holds the request that was SENT in a field, so
			// `resp.Request.Body` reaches a request-typed value one accessor
			// in — structurally identical to a house context's `c.Req.Body`.
			// What separates them is that `resp` is a local the handler built,
			// where a receiver or parameter is one it was given.
			method: "POST", path: "/items/echo", want: "",
			why: "the request we sent is not the request we are serving",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := opFor(out.Paths[tc.path], tc.method)
			if op == nil {
				t.Fatalf("%s %s missing; have %v", tc.method, tc.path, mapPathKeys(out.Paths))
			}

			if tc.want == "" {
				if op.RequestBody != nil {
					t.Errorf("%s %s documents a request body (%s), want none — %s",
						tc.method, tc.path, requestBodyRef(op), tc.why)
				}
				return
			}

			if op.RequestBody == nil {
				t.Fatalf("%s %s documents no request body, want one naming %q — %s",
					tc.method, tc.path, tc.want, tc.why)
			}
			if ref := requestBodyRef(op); !strings.HasSuffix(ref, "_"+tc.want) {
				t.Errorf("%s %s request body = %q, want it to name %q — %s",
					tc.method, tc.path, ref, tc.want, tc.why)
			}
		})
	}

	// The provider's type is not this service's business at all: with nothing
	// referencing it, it must not reach the served document either.
	for name := range out.Components.Schemas {
		if strings.HasSuffix(name, "_providerReply") {
			t.Errorf("component %q is a third-party client's reply type and is published in the spec", name)
		}
	}
}

// requestBodyRef renders an operation's JSON request-body schema for assertions
// and failure output: the $ref when there is one, else a short description of
// what stands in its place.
func requestBodyRef(op *intspec.Operation) string {
	if op == nil || op.RequestBody == nil {
		return "<none>"
	}
	mt, ok := op.RequestBody.Content["application/json"]
	if !ok {
		keys := make([]string, 0, len(op.RequestBody.Content))
		for k := range op.RequestBody.Content {
			keys = append(keys, k)
		}
		return "<no application/json, have " + strings.Join(keys, ",") + ">"
	}
	if mt.Schema == nil {
		return "<no schema>"
	}
	if mt.Schema.Ref == "" {
		return "<inline " + mt.Schema.Type + ">"
	}
	return mt.Schema.Ref
}

// TestTestdata_OutboundDecodeWrapperGin is the same rule under a different
// router, because none of it is chi's: derivation and the source check are
// shared, and only the configured requestContext differs (golden rule #5).
//
// The house context here holds a *gin.Context, so the request sits two
// accessors from the root (`c.G.Request.Body`) rather than one.
func TestTestdata_OutboundDecodeWrapperGin(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "outbound_decode_wrapper_gin", intspec.DefaultGinConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	cases := []struct {
		name         string
		method, path string
		want         string
		why          string
	}{
		{
			name:   "house wrapper over gin's context",
			method: "POST", path: "/settings", want: "Settings",
			why: "c.G.Request.Body is this request's body, two accessors from the root",
		},
		{
			name:   "outbound client with the same shape",
			method: "POST", path: "/contacts/sync", want: "",
			why: "the only decode reachable is an *http.Response",
		},
		{
			name:   "both, in one handler",
			method: "PUT", path: "/settings", want: "Settings",
			why: "the handler's own body must not be displaced by an outbound decode",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			op := opFor(out.Paths[tc.path], tc.method)
			if op == nil {
				t.Fatalf("%s %s missing; have %v", tc.method, tc.path, mapPathKeys(out.Paths))
			}
			if tc.want == "" {
				if op.RequestBody != nil {
					t.Errorf("%s %s documents a request body (%s), want none — %s",
						tc.method, tc.path, requestBodyRef(op), tc.why)
				}
				return
			}
			if op.RequestBody == nil {
				t.Fatalf("%s %s documents no request body, want one naming %q — %s",
					tc.method, tc.path, tc.want, tc.why)
			}
			if ref := requestBodyRef(op); !strings.HasSuffix(ref, "_"+tc.want) {
				t.Errorf("%s %s request body = %q, want it to name %q — %s",
					tc.method, tc.path, ref, tc.want, tc.why)
			}
		})
	}

	for name := range out.Components.Schemas {
		if strings.HasSuffix(name, "_providerReply") {
			t.Errorf("component %q is a third-party client's reply type and is published in the spec", name)
		}
	}
}
