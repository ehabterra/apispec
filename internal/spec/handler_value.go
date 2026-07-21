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
	"maps"
	"slices"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// Handler-value resolution (issue #204), shared by both tracker engines so they
// cannot drift: a route registered with a handler *value* rather than a func
// names no method anywhere in the registration, so the framework's handler
// interface supplies it.

// ambiguousConcreteSet returns the concrete types assigned to an interface-typed
// body when there is more than one — the case where narrowing is impossible but
// `oneOf` can still say exactly what the payload may be (issue #201).
//
// It mirrors resolveTypeOrigin's argument handling: a request body is decoded
// into a pointer (`Decode(&v)`), so the variable is the unary's operand, while a
// response encodes the value itself.
func (r *BasePatternMatcher) ambiguousConcreteSet(arg *metadata.CallArgument, node TrackerNodeInterface, originalType string) []string {
	if arg == nil || node == nil {
		return nil
	}
	target := arg
	if target.GetKind() == metadata.KindUnary && target.X != nil {
		target = target.X
	}
	if target.GetKind() != metadata.KindIdent && target.GetKind() != metadata.KindFuncLit {
		return nil
	}
	set := r.concreteSetFromEnclosingFunc(target, node.GetEdge(), originalType)
	if len(set) < 2 {
		return nil
	}
	return set
}

// oneOfSchemaFor builds a `oneOf` schema from a set of concrete types, or
// returns nil when there is nothing polymorphic to express (issue #201).
//
// This is the ambiguous branch of interface resolution. When a handler assigns
// more than one concrete type to an interface-typed variable, narrowing to one
// of them would be a guess, but falling back to the bare interface emits
// `{type: object}` — a schema that describes nothing. `oneOf: [Cat, Dog]` states
// exactly what is known, so it is the stronger form of "honest over wrong"
// (golden rule #7), not a relaxation of it.
//
// The set is the types assigned *in this handler*, deliberately not every
// recorded implementer of the interface: enumerating implementers the handler
// never assigns would over-claim.
//
// Each member is mapped through mapGoTypeToOpenAPISchema so it registers as a
// component and the `$ref`s resolve; a member that maps to nothing is dropped,
// and if fewer than two survive there is no polymorphism to express.
func oneOfSchemaFor(usedTypes map[string]*Schema, concretes []string, meta *metadata.Metadata, cfg *APISpecConfig) *Schema {
	if len(concretes) < 2 {
		return nil
	}
	members := make([]*Schema, 0, len(concretes))
	for _, ct := range concretes {
		schema, _ := mapGoTypeToOpenAPISchema(usedTypes, ct, meta, cfg, nil)
		if schema == nil {
			continue
		}
		members = append(members, schema)
	}
	if len(members) < 2 {
		return nil
	}
	return &Schema{OneOf: members}
}

// handlerMethodKeys returns the base IDs ("pkg.Type.Method") of the configured
// handler methods declared by the type (pkg, name), fanning an interface out to
// its implementers.
//
// Resolution is honest in both directions: a concrete type contributes a key
// only for a method it actually declares, and an interface contributes only its
// recorded implementers — never a same-named method picked from elsewhere
// (golden rules #7/#9). Frameworks whose handlers are plain func types pass no
// methods and get no keys.
func handlerMethodKeys(meta *metadata.Metadata, handlerMethods []string, pkg, name string) []string {
	if meta == nil || len(handlerMethods) == 0 || pkg == "" || name == "" {
		return nil
	}
	typ := findType(meta, pkg, name)
	if typ == nil {
		// An interface declared outside the analyzed set (net/http.Handler) has
		// no Type entry to carry ImplementedBy, so the relation is read from the
		// concrete side — the Implements facts recorded by
		// analyzeExternalInterfaceImplementations (issue #178).
		var out []string
		for _, impl := range implementersOfExternal(meta, pkg+"."+name) {
			implPkg, implName, ok := splitTypeKey(impl)
			if !ok {
				continue
			}
			out = append(out, handlerMethodKeys(meta, handlerMethods, implPkg, implName)...)
		}
		return out
	}
	if getStringFromPool(meta, typ.Kind) == "interface" {
		var out []string
		for _, implIdx := range typ.ImplementedBy {
			implPkg, implName, ok := splitTypeKey(getStringFromPool(meta, implIdx))
			if !ok {
				continue
			}
			out = append(out, handlerMethodKeys(meta, handlerMethods, implPkg, implName)...)
		}
		return out
	}
	var out []string
	for _, method := range handlerMethods {
		for i := range typ.Methods {
			if getStringFromPool(meta, typ.Methods[i].Name) == method {
				out = append(out, pkg+"."+name+"."+method)
				break
			}
		}
	}
	return out
}

// handlerValueTypeOf returns the named type of an argument that is a handler
// *value*, or ("", "") when the argument is a func/method value (whose type is a
// signature, resolved by the method-value paths instead) or is untyped.
func handlerValueTypeOf(arg *metadata.CallArgument) (pkg, name string) {
	if arg == nil {
		return "", ""
	}
	// A func signature has to be rejected up front: the type model has no
	// function kind (a signature is "otherwise opaque" KindNamed), so TypeRef
	// splits "func(w http.ResponseWriter, r *http.Request)" at its last dot and
	// yields a plausible-looking but meaningless pkg/name pair. findType would
	// then simply miss, but resolving a garbage type is not something to rely on.
	// The prefix check is how classifyArgument already distinguishes the two.
	if strings.HasPrefix(arg.GetType(), "func(") || strings.HasPrefix(arg.GetType(), "func[") {
		return "", ""
	}
	core := arg.TypeRef().Core()
	if !core.IsNamed() || core.Pkg == "" || core.Name == "" {
		return "", ""
	}
	return core.Pkg, core.Name
}

// splitTypeKey splits an "import/path.Type" key. Package paths contain dots
// (github.com/...), so the split is on the LAST dot, which is the type boundary.
func splitTypeKey(key string) (pkg, name string, ok bool) {
	i := strings.LastIndexByte(key, '.')
	if i < 0 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

// attachHandlerValueChildren is the eager tree's counterpart to LazyTree's
// handlerValueKeys expansion: it hangs the handler method's body under the
// argument node so the route's responses, params and request body are reachable.
//
// The lazy tree expands by *key* and lets edgesFor find the bodies; the eager
// tree materializes nodes up front, so the matching edges are looked up here —
// every edge whose CALLER is the resolved handler method, which is exactly what
// the existing method-value branch does for a selector argument.
func attachHandlerValueChildren(
	tree *TrackerTree,
	meta *metadata.Metadata,
	argNode *TrackerNode,
	arg *metadata.CallArgument,
	visited map[string]int,
	assignmentIndex *assigmentIndexMap,
	limits metadata.TrackerLimits,
) {
	if tree == nil || argNode == nil || len(tree.handlerMethods) == 0 {
		return
	}
	pkg, name := handlerValueTypeOf(arg)
	keys := handlerMethodKeys(meta, tree.handlerMethods, pkg, name)
	if len(keys) == 0 {
		return
	}
	for _, key := range keys {
		methodPkgType, methodName, ok := splitTypeKey(key)
		if !ok {
			continue
		}
		keyPkg, recvType, ok := splitTypeKey(methodPkgType)
		if !ok {
			continue
		}
		nameIdx := meta.StringPool.Get(methodName)
		pkgIdx := meta.StringPool.Get(keyPkg)
		recvIdx := meta.StringPool.Get(recvType)
		starRecvIdx := meta.StringPool.Get("*" + recvType)
		for i := range meta.CallGraph {
			e := &meta.CallGraph[i]
			if e.Caller.Name != nameIdx || e.Caller.Pkg != pkgIdx ||
				(e.Caller.RecvType != recvIdx && e.Caller.RecvType != starRecvIdx) {
				continue
			}
			if child := NewTrackerNode(tree, meta, argNode.Key(), e.Callee.ID(), e, nil, visited, assignmentIndex, limits); child != nil {
				argNode.AddChild(child)
			}
		}
	}
}

// implementersOfExternal returns the "pkg.Type" keys of every recorded type
// implementing the given interface key ("net/http.Handler"), read from the
// concrete side's Implements facts (issue #178).
//
// Interfaces declared outside the analyzed set never become Type entries, so
// they carry no ImplementedBy list to read the relation from — the concrete
// types are the only place the fact is recorded. Results are sorted: they feed
// tree expansion, whose order must not vary between runs (golden rule #1).
func implementersOfExternal(meta *metadata.Metadata, ifaceKey string) []string {
	if meta == nil || ifaceKey == "" {
		return nil
	}
	want := meta.StringPool.Get(ifaceKey)
	var out []string
	for _, pkgName := range slices.Sorted(maps.Keys(meta.Packages)) {
		p := meta.Packages[pkgName]
		for _, typeName := range slices.Sorted(maps.Keys(p.Types)) {
			if slices.Contains(p.Types[typeName].Implements, want) {
				out = append(out, pkgName+"."+typeName)
			}
		}
	}
	return out
}
