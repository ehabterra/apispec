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

import (
	"go/constant"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// longRoute is past go/constant's 72-character truncation point.
const longRoute = "/_apis/pipelines/workflows/{run_id}/jobs/{job_id}/steps/{step_index}/summary"

// Variable.Value is the initializer RENDERED, and that is a value only when the
// initializer was one. Returning it regardless put a rendered Go expression
// into the document as a parameter name (issue #452).
func TestDeclaredValue(t *testing.T) {
	pool := metadata.NewStringPool()
	cp := &ContextProviderImpl{meta: &metadata.Metadata{StringPool: pool}}

	cases := []struct {
		name     string
		variable *metadata.Variable
		want     string
		ok       bool
	}{
		{
			name:     "a literal initializer is a value",
			variable: &metadata.Variable{Value: pool.Get(`"X-Thing"`), ValueKind: pool.Get(metadata.KindLiteral)},
			want:     "X-Thing", ok: true,
		},
		{
			name: "a call initializer is not",
			variable: &metadata.Variable{
				Value:     pool.Get(`func(s string) pkg.k(CFG).MustString(X-Default)`),
				ValueKind: pool.Get(metadata.KindCall),
			},
			want: "", ok: false,
		},
		{
			// Knowable in principle, but not from the flattened rendering —
			// and "X- + Joined" is not a header anyone can send.
			name: "a binary expression is not",
			variable: &metadata.Variable{
				Value: pool.Get(`X- + Joined`), ValueKind: pool.Get(metadata.KindBinary),
			},
			want: "", ok: false,
		},
		{
			name: "another name is not",
			variable: &metadata.Variable{
				Value: pool.Get(`Other`), ValueKind: pool.Get(metadata.KindIdent),
			},
			want: "", ok: false,
		},
		{
			// A computed constant has no literal to read, and go/types has
			// already evaluated it.
			name:     "a computed constant answers when there is no literal",
			variable: &metadata.Variable{Value: pool.Get(`A + B`), ValueKind: pool.Get(metadata.KindBinary), ComputedValue: "X-Computed"},
			want:     "X-Computed", ok: true,
		},
		{
			// The LITERAL is preferred over the computed value, because a
			// go/constant renders through String(), which truncates past 72
			// characters — on gitea that shortened a 76-character route const
			// and moved an endpoint to a path no client could call.
			name: "a long literal is not truncated by the computed form",
			variable: &metadata.Variable{
				Value:         pool.Get(`"` + longRoute + `"`),
				ValueKind:     pool.Get(metadata.KindLiteral),
				ComputedValue: constant.MakeString(longRoute),
			},
			want: longRoute, ok: true,
		},
		{
			// And when only the computed form exists, it is read without
			// truncating either.
			name: "a long computed string is read in full",
			variable: &metadata.Variable{
				ValueKind:     pool.Get(metadata.KindBinary),
				ComputedValue: constant.MakeString(longRoute),
			},
			want: longRoute, ok: true,
		},
		{
			// Metadata written before ValueKind existed: the rendering is all
			// there is, and regressing every deserialised metadata.yaml at once
			// would be worse than the shapes this declines.
			name:     "no recorded kind keeps resolving",
			variable: &metadata.Variable{Value: pool.Get(`"X-Legacy"`)},
			want:     "X-Legacy", ok: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := cp.declaredValue(tc.variable)
			if got != tc.want || ok != tc.ok {
				t.Errorf("declaredValue = (%q, %v), want (%q, %v)", got, ok, tc.want, tc.ok)
			}
		})
	}
}
