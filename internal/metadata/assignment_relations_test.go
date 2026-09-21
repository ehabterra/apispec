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

package metadata

import (
	"go/ast"
	"go/types"
	"testing"
)

func relationsFor(t *testing.T, src string) (*Metadata, map[AssignmentKey]*AssignmentLink) {
	t.Helper()
	file, info, fset := sweepTypeCheck(t, src)
	meta := GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"sweep.go": file}},
		map[*ast.File]*types.Info{file: info},
		map[string]string{"sweep.go": "p"},
		fset,
	)
	return meta, meta.BuildAssignmentRelationships()
}

// `v := call()` links v to its producing call in EVERY function, not only main.
// A handler is registered as a value and never called, so the callee-body path
// never saw its assignments, and a helper handed one of its variables had no
// producer to bind to (issue #544).
func TestAssignmentRelationsCoverUncalledFunctions(t *testing.T) {
	meta, rels := relationsFor(t, `package p

type Header map[string][]string

type Writer struct{}

func (Writer) Header() Header { return nil }

func declare(h Header) {}

type reports struct{}
type invoices struct{}

// Never called: registered as a value, like every handler.
func handler(w Writer) {
	h := w.Header()
	declare(h)
}

func (reports) export(w Writer) {
	h := w.Header()
	declare(h)
}

func (invoices) export(w Writer) {
	h := w.Header()
	declare(h)
}
`)
	found := map[string]bool{}
	for key, rel := range rels {
		if key.Name != "h" {
			continue
		}
		if name := meta.StringPool.GetString(rel.Edge.Callee.Name); name != "Header" {
			t.Errorf("%+v links h to %q, want its producing Header() call", key, name)
		}
		found[key.Container] = true
	}
	for _, want := range []string{"handler", "reports.export", "invoices.export"} {
		if !found[want] {
			t.Errorf("no relation for h in %s; have %v", want, found)
		}
	}
}

// An assignment with no call on its right-hand side is not handed to the next
// call the walk meets. `c := &Ctx{}` followed by `c.Do()` recorded c.Do as the
// producer of c, which — once every function's relations were read — made the
// call its own producer and cycled it out of the tree.
func TestPendingAssignmentDoesNotLeakToNextCall(t *testing.T) {
	meta, _ := relationsFor(t, `package p

type Ctx struct{}

func (c *Ctx) Do() {}

func handler() {
	c := &Ctx{}
	c.Do()
}
`)
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		if meta.StringPool.GetString(e.Callee.Name) == "Do" && e.CalleeRecvVarName != "" {
			t.Errorf("c.Do() recorded as the producer of %q", e.CalleeRecvVarName)
		}
	}
}
