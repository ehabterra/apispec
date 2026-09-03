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
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// paramEdge builds a call edge whose parameters are bound by NAME, which is how
// the diagram's labels are produced.
func paramEdge(meta *metadata.Metadata, params map[string]string) *metadata.CallGraphEdge {
	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: meta, Name: meta.StringPool.Get("main"), Pkg: meta.StringPool.Get("app"), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: meta.StringPool.Get("HandleFunc"), Pkg: meta.StringPool.Get("net/http"), RecvType: -1},
	}
	edge.ParamArgMap = map[string]metadata.CallArgument{}
	for name, value := range params {
		arg := metadata.NewCallArgument(meta)
		arg.SetKind(metadata.KindLiteral)
		arg.SetValue(`"` + value + `"`)
		arg.Type = meta.StringPool.Get("string")
		edge.ParamArgMap[name] = *arg
	}
	return edge
}

// TestExtractParameterInfoIsOrdered pins the ordering of the diagram's
// parameter labels (issue #443).
//
// They were produced by ranging the edge's ParamArgMap, so two generations of
// an unchanged project emitted the same diagram with two labels swapped — the
// spec has been deterministic since #340 precisely so it can be committed and
// diffed, and the diagram is an output too.
func TestExtractParameterInfoIsOrdered(t *testing.T) {
	meta := newTestMeta()
	edge := paramEdge(meta, map[string]string{
		"pattern": "/item",
		"handler": "itemHandler",
		"zzz":     "last",
		"aaa":     "first",
	})

	wantTypes := []string{"aaa:string", "handler:string", "pattern:string", "zzz:string"}
	wantValues := []string{"aaa: first", "handler: itemHandler", "pattern: /item", "zzz: last"}

	// Repeated because Go randomises map iteration per range: one pass could
	// pass by luck, twenty could not.
	for i := 0; i < 20; i++ {
		types, values := extractParameterInfo(edge)
		if !reflect.DeepEqual(types, wantTypes) {
			t.Fatalf("run %d: param types = %v, want %v", i, types, wantTypes)
		}
		if !reflect.DeepEqual(values, wantValues) {
			t.Fatalf("run %d: param values = %v, want %v", i, values, wantValues)
		}
	}
}

// TestDrawCallGraphIsDeterministic is the guard one level up, over the drawing
// that actually emits these labels: `--diagram` renders the CALL GRAPH, and its
// per-node call paths are where the parameter values appear.
//
// Marshalled repeatedly rather than compared field by field, because the point
// is the bytes a user commits — and it would catch a future map range anywhere
// in the payload, not only the one #443 was about.
func TestDrawCallGraphIsDeterministic(t *testing.T) {
	meta := newTestMeta()
	meta.CallGraph = []metadata.CallGraphEdge{
		*paramEdge(meta, map[string]string{
			"pattern": "/item",
			"handler": "itemHandler",
			"mw":      "auth",
			"opts":    "defaults",
		}),
	}
	meta.BuildCallGraphMaps()

	var first string
	for i := 0; i < 20; i++ {
		payload, err := json.Marshal(DrawCallGraphCytoscape(meta))
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		if i == 0 {
			first = string(payload)
			if !strings.Contains(first, "pattern: /item") {
				t.Fatalf("the fixture does not reach the parameter labels; payload=%s", first)
			}
			continue
		}
		if string(payload) != first {
			t.Fatalf("run %d produced a different diagram payload than run 0", i)
		}
	}
}
