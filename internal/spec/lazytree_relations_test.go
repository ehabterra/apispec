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

func TestLazyTreeBuildRelationsScopesReceiverClaimsToAssignmentFunction(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	pkg := pool.Get("example")
	routesFunc := pool.Get("routes.func1")
	middlewareFunc := pool.Get("middleware.func1")

	// Pooled string fields use -1 as the "unset" sentinel; 0 is a valid index
	// (the first interned string), so leaving them at Go's zero value would
	// make Call.ID()/BaseID() resolve unset fields to a real string.
	call := func(name string, position string) metadata.Call {
		return metadata.Call{
			Meta:         meta,
			Name:         pool.Get(name),
			Pkg:          pkg,
			Position:     pool.Get(position),
			RecvType:     -1,
			Scope:        -1,
			SignatureStr: -1,
		}
	}
	assignment := func(variable string, function int, concreteType string) metadata.Assignment {
		return metadata.Assignment{
			VariableName: pool.Get(variable),
			Pkg:          pkg,
			ConcreteType: pool.Get(concreteType),
			Func:         function,
			Position:     -1,
			Scope:        -1,
		}
	}

	meta.CallGraph = []metadata.CallGraphEdge{
		{
			Caller: call("routes.func1", "1"),
			Callee: call("middleware", "2"),
			AssignmentMap: map[string][]metadata.Assignment{
				"r": {assignment("r", middlewareFunc, "*net/http.Request")},
			},
		},
		{
			Caller:        call("routes.func1", "3"),
			Callee:        call("Get", "4"),
			CalleeVarName: "r",
		},
		{
			Caller: call("routes.func1", "5"),
			Callee: call("Group", "6"),
			AssignmentMap: map[string][]metadata.Assignment{
				"g": {assignment("g", routesFunc, "chi.Router")},
			},
		},
		{
			Caller:        call("routes.func1", "7"),
			Callee:        call("Get", "8"),
			CalleeVarName: "g",
		},
	}
	meta.BuildCallGraphMaps()

	tree := &LazyTree{meta: meta}
	tree.buildRelations()

	if tree.claimed[&meta.CallGraph[1]] {
		t.Fatal("cross-scope middleware assignment claimed the router receiver edge")
	}
	if !tree.claimed[&meta.CallGraph[3]] {
		t.Fatal("same-scope group assignment did not claim its receiver edge")
	}
}
