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
	"sort"

	"github.com/ehabterra/apispec/internal/metadata"
)

// maxMapperDepth bounds recursion through nested error mappers (mapper calling
// a per-status helper mapper, …). Deep enough for the real MapError→mapAsXxx
// shape, small enough to stay cheap.
const maxMapperDepth = 8

// statusesFromMapperField resolves `x.Field` where x is produced by a *mapper*
// function whose return statements set that struct field across branches — the
// write-side status-mapper pattern (issue #187):
//
//	api := MapError(err)                     // api produced by a mapper
//	http.Error(w, api.Message, api.Status)   // status = api.Status
//
//	func MapError(err error) APIError {
//	    if api, ok := mapAs404(err); ok { return api }   // -> 404 (via helper)
//	    ...
//	    return APIError{http.StatusInternalServerError, "internal error"} // -> 500
//	}
//
// It enumerates the concrete statuses the field takes across every return of the
// mapper — recursing through helper mappers (`return api` where `api` came from
// `mapAsXxx(err)`) and reading struct-literal fields (keyed or positional) — and
// unions them. residue is true when a branch sets the field to a non-constant
// value, so the caller keeps an honest `default`. Returns nil when the base is
// not a mapper-produced struct field or nothing resolves.
//
// Depends on #192: enumeration needs every return statement (Function.Returns),
// not just the greatest-arity one.
func (r *ResponsePatternMatcherImpl) statusesFromMapperField(arg *metadata.CallArgument, node TrackerNodeInterface) (codes []int, residue bool) {
	if arg == nil || arg.GetKind() != metadata.KindSelector || arg.X == nil || arg.Sel == nil {
		return nil, false
	}
	fieldName := arg.Sel.GetName()
	if fieldName == "" {
		return nil, false
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return nil, false
	}

	// The selector base up to its producer: either the mapper call itself
	// (inline `MapError(err).Status`) or a local assigned from it
	// (`api := MapError(err); … api.Status`).
	base, baseNode := resolveArgThroughParams(arg.X, node)
	if base == nil || baseNode == nil || baseNode.GetEdge() == nil {
		return nil, false
	}

	var mapperFunc, mapperPkg string
	switch base.GetKind() {
	case metadata.KindCall:
		// Same-package `MapError(...)` (ident Fun) or cross-package
		// `pkg.MapError(...)` (selector Fun — name lives in .Sel).
		mapperFunc = calleeNameOf(base.Fun)
	case metadata.KindIdent:
		as := assignmentsAt(impl, baseNode.GetEdge(), base.GetName())
		if len(as) == 0 || as[len(as)-1].Value.GetKind() != metadata.KindCall {
			return nil, false
		}
		mapperFunc = as[len(as)-1].CalleeFunc
		mapperPkg = as[len(as)-1].CalleePkg
	default:
		return nil, false
	}
	if mapperFunc == "" {
		return nil, false
	}

	mapper := findFunctionByName(impl.meta, mapperPkg, mapperFunc)
	if mapper == nil {
		return nil, false
	}

	set := map[int]bool{}
	residue = r.returnFieldStatuses(impl, mapper, fieldName, set, map[string]bool{}, 0)
	if len(set) == 0 {
		return nil, residue
	}
	codes = make([]int, 0, len(set))
	for c := range set {
		codes = append(codes, c)
	}
	sort.Ints(codes)
	return codes, residue
}

// returnFieldStatuses adds, to set, the concrete statuses fieldName takes across
// every return of fn, recursing through helper mappers. residue is true when a
// branch sets the field to a non-constant value. seen guards against recursive
// mappers; depth bounds the chain length.
func (r *ResponsePatternMatcherImpl) returnFieldStatuses(impl *ContextProviderImpl, fn *metadata.Function, fieldName string, set map[int]bool, seen map[string]bool, depth int) (residue bool) {
	if fn == nil {
		return false
	}
	if depth > maxMapperDepth {
		return true // gave up before resolving — honest residue
	}
	fnID := impl.GetString(fn.Pkg) + "." + impl.GetString(fn.Name)
	if seen[fnID] {
		return false // already accounted for on the first visit
	}
	seen[fnID] = true

	for i := range fn.Returns {
		for j := range fn.Returns[i] {
			residue = r.fieldStatusOfValue(impl, &fn.Returns[i][j], fieldName, fn, set, seen, depth) || residue
		}
	}
	return residue
}

// fieldStatusOfValue resolves fieldName on a single returned value: a struct
// composite (read the field directly), a returned local (resolve its assignment,
// recursing into a helper mapper), or a direct helper call. A value that has no
// such field (a bool/string tuple element, an empty struct) contributes nothing
// and is *not* residue.
func (r *ResponsePatternMatcherImpl) fieldStatusOfValue(impl *ContextProviderImpl, val *metadata.CallArgument, fieldName string, scope *metadata.Function, set map[int]bool, seen map[string]bool, depth int) (residue bool) {
	if val == nil {
		return false
	}

	// A struct composite literal (possibly behind `&`): read the field.
	if lit := compositeLitOf(val); lit != nil {
		fv := structFieldValue(impl, lit, fieldName, scope)
		if fv == nil {
			return false // field not set here (e.g. APIError{}) — no status, not residue
		}
		if s, ok := r.statusCodeOfValue(fv, impl); ok {
			set[s] = true
			return false
		}
		return true // field set to a non-constant — honest residue
	}

	switch val.GetKind() {
	case metadata.KindIdent:
		// A returned local (`return api`): union over every assignment to it in
		// this scope — each a helper-mapper call or a struct composite.
		for i := range scope.AssignmentMap[val.GetName()] {
			a := &scope.AssignmentMap[val.GetName()][i]
			switch a.Value.GetKind() {
			case metadata.KindCall:
				helper := findFunctionByName(impl.meta, a.CalleePkg, a.CalleeFunc)
				residue = r.returnFieldStatuses(impl, helper, fieldName, set, seen, depth+1) || residue
			default:
				residue = r.fieldStatusOfValue(impl, &a.Value, fieldName, scope, set, seen, depth+1) || residue
			}
		}
		return residue
	case metadata.KindCall:
		// A returned helper call directly (`return mapAsXxx(err)` or
		// `return pkg.mapAsXxx(err)`). The callee package is usually the mapper's
		// own; findFunctionByName falls back to an all-packages name search.
		name := calleeNameOf(val.Fun)
		if name == "" {
			return false
		}
		helper := findFunctionByName(impl.meta, impl.GetString(scope.Pkg), name)
		return r.returnFieldStatuses(impl, helper, fieldName, set, seen, depth+1)
	}
	return false
}

// structFieldValue returns the value a struct composite literal assigns to
// fieldName, handling both keyed (`T{Field: v}`) and positional (`T{v, …}`)
// forms. For the positional form it indexes the field via the type's declared
// field order. Returns nil when the field is absent (including an empty `T{}`).
func structFieldValue(impl *ContextProviderImpl, lit *metadata.CallArgument, fieldName string, scope *metadata.Function) *metadata.CallArgument {
	// Keyed form: any KeyValue element means the whole literal is keyed.
	for _, elt := range lit.Args {
		if elt != nil && elt.GetKind() == metadata.KindKeyValue {
			return keyedFieldValue(lit, fieldName)
		}
	}

	// Positional form: index fieldName through the type definition.
	if lit.X == nil {
		return nil
	}
	pkg := impl.GetString(scope.Pkg)
	typeName := lit.X.GetName()
	if lit.X.GetKind() == metadata.KindSelector && lit.X.X != nil && lit.X.Sel != nil {
		pkg = lit.X.X.GetName() // pkg-qualified type: use the selector's package
		typeName = lit.X.Sel.GetName()
	}
	typ := findType(impl.meta, pkg, typeName)
	if typ == nil {
		typ = findTypeAnywhere(impl.meta, typeName)
	}
	if typ == nil {
		return nil
	}
	idx := -1
	for i := range typ.Fields {
		if impl.GetString(typ.Fields[i].Name) == fieldName {
			idx = i
			break
		}
	}
	if idx < 0 || idx >= len(lit.Args) {
		return nil
	}
	return lit.Args[idx]
}

// keyedFieldValue returns the value for fieldName in a keyed composite literal,
// or nil when absent.
func keyedFieldValue(lit *metadata.CallArgument, fieldName string) *metadata.CallArgument {
	for _, elt := range lit.Args {
		if elt != nil && elt.GetKind() == metadata.KindKeyValue &&
			elt.X != nil && elt.X.GetName() == fieldName {
			return elt.Fun
		}
	}
	return nil
}

// findTypeAnywhere locates a type by bare name across all packages, sorted for
// determinism. Used when a positional composite's package can't be pinned to the
// scope's package (a cross-package struct).
func findTypeAnywhere(meta *metadata.Metadata, typeName string) *metadata.Type {
	if meta == nil || typeName == "" {
		return nil
	}
	pkgs := make([]string, 0, len(meta.Packages))
	for p := range meta.Packages {
		pkgs = append(pkgs, p)
	}
	sort.Strings(pkgs)
	for _, p := range pkgs {
		if t := findType(meta, p, typeName); t != nil {
			return t
		}
	}
	return nil
}
