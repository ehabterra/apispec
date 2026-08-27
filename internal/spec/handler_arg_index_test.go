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

package spec

import "testing"

// TestHandlerArgIndexFor covers issue #386's resolver. The two directions are
// the point: gin and fiber put the endpoint handler LAST behind any per-route
// middleware, while echo puts it first and lets the middleware follow, so a
// pattern that opts out must keep reading its fixed index no matter how many
// arguments the call carries.
func TestHandlerArgIndexFor(t *testing.T) {
	cases := []struct {
		name    string
		pattern RoutePattern
		nargs   int
		want    int
		wantOK  bool
	}{
		// Variadic chain (gin, fiber).
		{"chain: no middleware", RoutePattern{HandlerArgIndex: 1, HandlerArgFromEnd: true}, 2, 1, true},
		{"chain: one middleware", RoutePattern{HandlerArgIndex: 1, HandlerArgFromEnd: true}, 3, 2, true},
		{"chain: two middlewares", RoutePattern{HandlerArgIndex: 1, HandlerArgFromEnd: true}, 4, 3, true},
		// A verb-carrying registrar whose chain starts later still never
		// resolves before its own index.
		{"chain: later start, no middleware", RoutePattern{HandlerArgIndex: 2, HandlerArgFromEnd: true}, 3, 2, true},
		{"chain: later start, one middleware", RoutePattern{HandlerArgIndex: 2, HandlerArgFromEnd: true}, 4, 3, true},

		// Fixed index (echo, chi, mux, net/http). Trailing middleware must NOT
		// pull the handler along: e.GET(path, h, mw1, mw2) keeps it at 1.
		{"fixed: exact arity", RoutePattern{HandlerArgIndex: 1}, 2, 1, true},
		{"fixed: trailing middleware ignored", RoutePattern{HandlerArgIndex: 1}, 4, 1, true},
		{"fixed: third argument", RoutePattern{HandlerArgIndex: 2}, 3, 2, true},

		// Too short to carry a handler, either way.
		{"short: fixed", RoutePattern{HandlerArgIndex: 1}, 1, 0, false},
		{"short: chain", RoutePattern{HandlerArgIndex: 1, HandlerArgFromEnd: true}, 1, 0, false},
		{"empty call", RoutePattern{HandlerArgIndex: 0, HandlerArgFromEnd: true}, 0, 0, false},
		// A negative index is not a "read from the end" spelling — it names no
		// argument, and must not index backwards into the slice.
		{"negative index", RoutePattern{HandlerArgIndex: -1}, 3, 0, false},
		// Nor does the variadic flag rescue it: resolving to the last argument
		// here would invent a handler for a pattern that declared none. Both
		// fields are user-editable, so this pairing is reachable.
		{"negative index with the variadic flag", RoutePattern{HandlerArgIndex: -1, HandlerArgFromEnd: true}, 3, 0, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tc.pattern.HandlerArgIndexFor(tc.nargs)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tc.wantOK)
			}
			if got != tc.want {
				t.Errorf("index = %d, want %d", got, tc.want)
			}
		})
	}
}
