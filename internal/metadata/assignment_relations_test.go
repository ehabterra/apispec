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

func metadataFor(t *testing.T, src string) *Metadata {
	t.Helper()
	file, info, fset := sweepTypeCheck(t, src)
	return GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"sweep.go": file}},
		map[*ast.File]*types.Info{file: info},
		map[string]string{"sweep.go": "p"},
		fset,
	)
}

// An assignment with no call on its right-hand side is not handed to the next
// call the walk meets. `c := &Ctx{}` followed by `c.Do()` recorded c.Do as the
// producer of c (issue #550).
func TestPendingAssignmentDoesNotLeakToNextCall(t *testing.T) {
	meta := metadataFor(t, `package p

type Ctx struct{}

func (c *Ctx) Do() {}

func handler() {
	c := &Ctx{}
	c.Do()
}

func main() { handler() }
`)
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		if meta.StringPool.GetString(e.Callee.Name) == "Do" && e.CalleeRecvVarName != "" {
			t.Errorf("c.Do() recorded as the producer of %q", e.CalleeRecvVarName)
		}
	}
}

// A variable assigned inside a function with no call behind it has no
// producer. It used to be linked to the INVOCATION of the function it is
// written in — for a handler closure, the route registration — which made the
// registration's whole subtree the variable's value (issue #550).
func TestAssignmentWithoutProducingCallHasNoRelation(t *testing.T) {
	meta := metadataFor(t, `package p

type Req struct{ Path string }

func save(string) {}

func register(r *Req) {
	name := r.Path
	save(name)
}

func main() { register(&Req{}) }
`)
	for key, rel := range meta.BuildAssignmentRelationships() {
		if key.Name != "name" {
			continue
		}
		t.Errorf("name is linked to %q, but no call produced it",
			meta.StringPool.GetString(rel.Edge.Callee.Name))
	}
}

// The case the fallback exists for keeps working: an assignment in the
// CALLER's own scope, recorded on the edge that produced it — `main`'s
// `g := newGroup()`, every variable of a tuple included.
func TestCallerScopeAssignmentKeepsItsProducer(t *testing.T) {
	meta := metadataFor(t, `package p

type Group struct{}

func newGroup() (*Group, error) { return &Group{}, nil }

func main() {
	g, err := newGroup()
	_, _ = g, err
}
`)
	linked := map[string]string{}
	for key, rel := range meta.BuildAssignmentRelationships() {
		linked[key.Name] = meta.StringPool.GetString(rel.Edge.Callee.Name)
	}
	for _, v := range []string{"g", "err"} {
		if linked[v] != "newGroup" {
			t.Errorf("%s linked to %q, want its producing newGroup() call; have %v", v, linked[v], linked)
		}
	}
}
