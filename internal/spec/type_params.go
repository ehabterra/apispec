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
	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// resolveTypeParam substitutes a TYPE PARAMETER with the type bound to it at
// this instantiation, and reports whether the type was a type parameter at all.
//
// A generic handler adapter — `HandleJSON[Req, Res any](fn func(context.Context,
// Req) (Res, error)) http.HandlerFunc`, the shape several Go HTTP toolkits ship
// and many projects hand-roll — writes its response by encoding a value whose
// declared type is the parameter `Res`. The request side already resolved,
// because `in` is a DECLARED VARIABLE of type Req; `out` is the result of
// calling a func-typed parameter, and nothing applied the binding to it. The
// type parameter's own name then reached the schema mapper, which emitted a
// component named after it: `..._Res`, `type: object`, "unresolved type" — and
// EVERY operation built through the adapter referenced that one component, so
// two endpoints with entirely different response types claimed the same schema
// (issue #367).
//
// The binding was never missing. At each registration the node's type-parameter
// map holds exactly the right instantiation — {Res: pkg.User} under one route,
// {Res: pkg.ListResponse} under the next — which is also why the operationId
// could print it. All that was needed was to look.
//
// Substitution is structural, through the type model rather than by string
// surgery (golden rule #2), so a parameter under a constructor comes out whole:
// `[]Res` becomes `[]pkg.User`, not `pkg.User`.
func resolveTypeParam(goType string, node TrackerNodeInterface, meta *metadata.Metadata) (string, bool) {
	if goType == "" || node == nil {
		return goType, false
	}
	ref := typemodel.Parse(goType)
	core := ref.Core()
	if core == nil || core.Name == "" {
		return goType, false
	}

	bound, ok := node.GetTypeParamMap()[core.Name]
	if !ok {
		// Not bound here. It is still a type parameter if the function that
		// wrote this value declares it as one, and that case must not reach the
		// mapper either — see unresolvedTypeParamType.
		if isDeclaredTypeParam(core.Name, node, meta) {
			return unresolvedTypeParamType, true
		}
		return goType, false
	}
	if bound == "" || bound == goType {
		return goType, false
	}

	boundCore := typemodel.Parse(bound).Core()
	if boundCore == nil || boundCore.Name == "" {
		return goType, false
	}
	// Clone before mutating: TypeRefs are shared (golden rule #2).
	out := ref.Clone()
	outCore := out.Core()
	outCore.Pkg = boundCore.Pkg
	outCore.Name = boundCore.Name
	outCore.Args = boundCore.Args
	return out.String(), true
}

// unresolvedTypeParamType is what an unresolved type parameter becomes: the
// honest general type, which the mapper renders INLINE as `type: object` with
// no $ref.
//
// A shared bogus $ref is strictly worse than an inline unknown, because it does
// not merely fail to describe the body — it asserts that two operations return
// the same type when they do not, and a consumer generating clients from that
// document gets one wrong Go type for both (golden rule #7).
const unresolvedTypeParamType = "any"

// isDeclaredTypeParam reports whether name is a type parameter declared by the
// function this node is expanding, or by one of its callers.
//
// The chain is walked because the value can be written deeper than the generic
// function itself: the adapter's `Res` is written inside the closure it returns,
// whose own record declares no type parameters at all.
func isDeclaredTypeParam(name string, node TrackerNodeInterface, meta *metadata.Metadata) bool {
	if name == "" || meta == nil {
		return false
	}
	for cur := node; cur != nil; cur = cur.GetParent() {
		edge := cur.GetEdge()
		if edge == nil {
			continue
		}
		for _, call := range []*metadata.Call{&edge.Caller, &edge.Callee} {
			fn := meta.FunctionInPackage(getString(meta, call.Pkg), getString(meta, call.Name))
			if fn == nil {
				continue
			}
			for _, tp := range fn.TypeParams {
				if tp == name {
					return true
				}
			}
		}
	}
	return false
}
