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

// TestLazyTreeCallArgumentIsItsOwnProducer covers issue #407's binding: a value
// created in an argument list — RegisterRouter(v1.Group("/mod")) — has no
// assignment for the ident path to trace, so the call itself is the producer.
//
// The claim that matters is not just that a key is produced but *which* key:
// the callee's receiver edges have to land under the node the group call
// materializes as, or they attach to a key no node ever has and the routes
// disappear instead of merely losing their prefix. So the resolved key is
// compared against the producing edge's own callee ID.
func TestLazyTreeCallArgumentIsItsOwnProducer(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	pkg := pool.Get("example")

	call := func(name, position string) metadata.Call {
		return metadata.Call{
			Meta: meta, Name: pool.Get(name), Pkg: pkg, Position: pool.Get(position),
			RecvType: -1, Scope: -1, SignatureStr: -1,
		}
	}
	// `v1.Group("/mod")` written straight into RegisterRouter's argument list.
	groupArg := metadata.CallArgument{
		Meta: meta, Kind: pool.Get(metadata.KindCall), Position: pool.Get("6"),
		Name: -1, Value: -1, Raw: -1, Pkg: -1, Type: -1,
		Fun: &metadata.CallArgument{
			Meta: meta, Kind: pool.Get(metadata.KindIdent), Name: pool.Get("Group"),
			Pkg: pkg, Value: -1, Raw: -1, Type: -1, Position: -1,
		},
	}

	meta.CallGraph = []metadata.CallGraphEdge{
		// The group call, which is also the argument below.
		{Caller: call("main", "5"), Callee: call("Group", "6")},
		// RegisterRouter(v1.Group("/mod")) — parameter rg bound to the call.
		{
			Caller:      call("main", "7"),
			Callee:      call("RegisterRouter", "8"),
			ParamArgMap: map[string]metadata.CallArgument{"rg": groupArg},
		},
		// rg.POST("/login", ...) inside RegisterRouter's body.
		{Caller: call("RegisterRouter", "8"), Callee: call("POST", "9"), CalleeVarName: "rg"},
	}
	meta.BuildCallGraphMaps()

	tree := &LazyTree{meta: meta}
	tree.buildRelations()

	producer := meta.CallGraph[0].Callee.ID()
	if got := groupArg.ID(); got != producer {
		t.Fatalf("argument resolves to %q, but the group call materializes as %q — "+
			"registrations would hang under a key no node has", got, producer)
	}
	kids := tree.receiverChildren[producer]
	if len(kids) != 1 || kids[0] != &meta.CallGraph[2] {
		t.Fatalf("the callee's registration did not become a child of the group call: %v", kids)
	}
	if !tree.claimed[&meta.CallGraph[2]] {
		t.Error("the registration was left unclaimed, so it is also walked without the group prefix")
	}
}
