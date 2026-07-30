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

	intspec "github.com/ehabterra/apispec/internal/spec"
)

// TestTestdata_ResolvedIfaceScope guards the interaction between a resolved
// (SSA+VTA) call graph and patterns that are scoped to an INTERFACE.
//
// The net/http param config documents `Header().Get(k)` as a request header and
// excludes the case where the header is the server's own response, by naming the
// interface the read comes from: `net/http.ResponseWriter`. Resolving the call
// graph replaces that interface with the concrete type that actually runs — the
// project's own `*recorder` here — so an exclusion that knows only the recorded
// receiver stops excluding, and the response header is documented as a request
// parameter: a better call graph producing a worse spec (issue #260).
//
// Measured on a real ~900-route project, the same shape (a render helper reading
// `respWriter.Header().Get("Content-Type")`) added the parameter to four
// operations.
//
// The fix is at the pattern layer: a rewritten call CARRIES the receiver it was
// written against alongside the concrete one, and receiver scoping accepts
// either (recvForms). This is the end-to-end guard for that rule — the spec must
// be identical with and without resolution here, because resolution changes
// which function runs, not what the source says.
func TestTestdata_ResolvedIfaceScope(t *testing.T) {
	const fixture = "resolved_iface_scope"

	headerParams := func(out *intspec.OpenAPISpec) []string {
		var names []string
		for _, item := range out.Paths {
			for _, op := range []*intspec.Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch} {
				if op == nil {
					continue
				}
				for _, p := range op.Parameters {
					if p.In == "header" {
						names = append(names, p.Name)
					}
				}
			}
		}
		return names
	}

	t.Run("syntactic", func(t *testing.T) {
		out := loadTestdata(t, fixture, nil)
		if got := headerParams(out); len(got) != 0 {
			t.Errorf("header parameters %v — every Header() call in this fixture is on the RESPONSE; none is a request parameter", got)
		}
	})

	t.Run("resolved", func(t *testing.T) {
		out := loadTestdataResolved(t, fixture, nil)
		if got := headerParams(out); len(got) != 0 {
			t.Errorf("header parameters %v with the resolved call graph — the response-writer exclusion is scoped to "+
				"net/http.ResponseWriter, so a rewritten receiver must still carry the interface it was written against (#260)", got)
		}
	})
}
