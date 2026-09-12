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
// surgery (golden rule #2), so constructors survive on BOTH sides: `[]Res`
// becomes `[]pkg.User` rather than `pkg.User`, and a `Res` bound to `[]User` —
// the ordinary list endpoint — stays an array rather than collapsing to the
// element.
func resolveTypeParam(goType string, node TrackerNodeInterface, meta *metadata.Metadata) (string, bool) {
	if goType == "" || node == nil {
		return goType, false
	}
	ref := typemodel.Parse(goType)
	core := paramOccurrence(ref)
	if core == nil || core.Name == "" {
		return goType, false
	}

	bound, ok := node.GetTypeParamMap()[core.Name]
	if !ok {
		// Not bound here. It is still a type parameter if the function that
		// wrote this value declares it as one, and that case must not reach the
		// mapper either — see unresolvedTypeParamType.
		if isDeclaredTypeParam(core.Name, node, meta) {
			return substituteCore(ref, typemodel.Parse(unresolvedTypeParamType)), true
		}
		return goType, false
	}
	if bound == "" || bound == goType {
		return goType, false
	}

	boundRef := typemodel.Parse(bound)
	if c := paramOccurrence(boundRef); c == nil || c.Name == "" {
		return goType, false
	}
	return substituteCore(ref, boundRef), true
}

// paramOccurrence returns the node a type parameter would occupy: the type with
// its constructors peeled off.
//
// typemodel's own Core() stops at a map, and the map case is not exotic — a
// handler answering `map[string]Res` is one `additionalProperties` away from
// the same defect, emitting a `$ref` to a component named after the parameter.
// The VALUE type is followed rather than the key, because a parameter used as a
// map key cannot reach an OpenAPI schema anyway: property names are strings.
func paramOccurrence(t *typemodel.TypeRef) *typemodel.TypeRef {
	for t != nil {
		switch t.Kind {
		case typemodel.KindPointer, typemodel.KindSlice, typemodel.KindArray,
			typemodel.KindChan, typemodel.KindMap:
			t = t.Elem
		default:
			return t
		}
	}
	return nil
}

// substituteCore replaces the type-parameter node inside ref with the WHOLE
// bound type, so constructors on both sides survive.
//
// Each side can carry its own, and grafting the binding's core alone dropped
// them: `Res` bound to `[]User` — an ordinary list endpoint — came out as
// `User`, documenting an object where the handler sends an array, and `[]Res`
// bound to `[]User` came out as `[]User` rather than `[][]User`. Core() returns
// a pointer INTO the tree, so overwriting the node it names keeps whatever the
// occurrence wrapped it in.
func substituteCore(ref, bound *typemodel.TypeRef) string {
	// Clone both: TypeRefs are shared, and the graft mutates (golden rule #2).
	out := ref.Clone()
	*paramOccurrence(out) = *bound.Clone()
	return out.String()
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
