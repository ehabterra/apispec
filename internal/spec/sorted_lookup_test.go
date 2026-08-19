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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestSortedAssignmentRelationshipsIsATotalOrder pins the tie-break that issue
// #340 turned on: one edge can produce several assignment links, so the
// producing callee ID alone does not order them, and the consumers write the
// result into last-write-wins indexes.
func TestSortedAssignmentRelationshipsIsATotalOrder(t *testing.T) {
	meta := newTestMeta()
	pool := meta.StringPool

	// One edge, four variables — every link shares a callee ID, so ordering is
	// decided entirely by the tie-break, and each pair below differs in exactly
	// one AssignmentKey field.
	assign := func(pkg, typ, fn string) []metadata.Assignment {
		return []metadata.Assignment{{Pkg: pool.Get(pkg), ConcreteType: pool.Get(typ), Func: pool.Get(fn)}}
	}
	meta.CallGraph = []metadata.CallGraphEdge{{
		Caller: metadata.Call{Meta: meta, Name: pool.Get("Setup"), Pkg: pool.Get("app"), RecvType: -1},
		Callee: metadata.Call{Meta: meta, Name: pool.Get("New"), Pkg: pool.Get("app/router"), RecvType: -1},
		AssignmentMap: map[string][]metadata.Assignment{
			"r": assign("app", "*Router", "Setup"),
			"s": assign("app", "*Router", "Setup"),   // differs by Name
			"t": assign("app", "*Engine", "Setup"),   // differs by Type
			"u": assign("app", "*Router", "Startup"), // differs by Container
			"v": assign("zzz", "*Router", "Setup"),   // differs by Pkg
		},
	}}

	want := []string{"Setup|app|r|*Router", "Setup|app|s|*Router", "Setup|app|t|*Engine",
		"Setup|zzz|v|*Router", "Startup|app|u|*Router"}
	key := func(l *metadata.AssignmentLink) string {
		k := l.AssignmentKey
		return k.Container + "|" + k.Pkg + "|" + k.Name + "|" + k.Type
	}

	// Repeated because the source is a map: each call sees a different input
	// order, which is exactly what an unstable sort turned into a different
	// result.
	for run := 0; run < 8; run++ {
		rels := sortedAssignmentRelationships(meta)
		if len(rels) != len(want) {
			t.Fatalf("run %d: got %d links, want %d", run, len(rels), len(want))
		}
		for i, rel := range rels {
			if got := key(rel); got != want[i] {
				t.Fatalf("run %d: link %d is %s, want %s", run, i, got, want[i])
			}
		}
	}
}

// TestSortedFileScansSkipNilFileEntries covers the lookups that answer from the
// first file declaring a name. They range the sorted file names, so a nil file
// entry — which a deserialised metadata.yaml may carry — sits in the middle of
// the scan and must be stepped over rather than dereferenced.
func TestSortedFileScansSkipNilFileEntries(t *testing.T) {
	meta := newTestMeta()
	pool := meta.StringPool

	const pkg = "example.com/app"
	handler := &metadata.Type{
		Methods: []metadata.Method{{
			Name:     pool.Get("Serve"),
			Receiver: pool.Get("*Handler"),
			AssignmentMap: map[string][]metadata.Assignment{
				"body": {{Pkg: pool.Get(pkg), ConcreteType: pool.Get("Payload")}},
			},
		}},
	}
	meta.Packages = map[string]*metadata.Package{
		pkg: {
			Files: map[string]*metadata.File{
				// Sorts first, and is nil: every scan below starts here.
				"a_generated.go": nil,
				"b_real.go": {
					Types:     map[string]*metadata.Type{"Handler": handler, "Payload": {}},
					Functions: map[string]*metadata.Function{"Route": {Name: pool.Get("Route")}},
					Variables: map[string]*metadata.Variable{
						"DefaultLimit": {Tok: pool.Get("const"), Type: pool.Get("int")},
					},
				},
			},
		},
	}

	if findFunction(meta, pkg, "Route") == nil {
		t.Error("findFunction skipped the real file after the nil entry")
	}
	if findType(meta, pkg, "Payload") == nil {
		t.Error("findType skipped the real file after the nil entry")
	}
	if findMethodByName(meta, pkg, "*Handler", "Serve") == nil {
		t.Error("findMethodByName skipped the real file after the nil entry")
	}
	if methodAssignmentMap(meta, pkg, "*Handler", "Serve", "body") == nil {
		t.Error("methodAssignmentMap skipped the real file after the nil entry")
	}

	cp := NewContextProvider(meta)
	arg := mkIdent(meta, "DefaultLimit", "")
	arg.Pkg = pool.Get(pkg)
	if got := constIdentDeclaredType(arg, cp); got != "int" {
		t.Errorf("constIdentDeclaredType = %q, want int (the nil file must be skipped)", got)
	}
}
