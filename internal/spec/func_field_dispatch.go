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
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// funcFieldDispatch resolves a call made *through a func-typed struct field* —
// `c.Action()` — to the functions that field actually holds (issue #143).
//
// This is the urfave/cli wiring style, and it is the whole reason apispec found
// zero routes in gitea: route registration is reachable from main only through a
// function value parked in a composite-literal field and invoked later by the
// library's dispatcher. There is no static call edge from main's subtree into
// the registering function, so tree expansion stopped at `app.Run(...)` and
// never saw a single route.
//
// The field is a dispatch point in exactly the way an interface method is, and
// it is resolved the same way: collect the concrete values recorded for it and
// expand into all of them. Several commands assigning the same field is not
// ambiguity to be guessed at — each is genuinely invoked for its own
// subcommand, so the union of their registrations is the binary's surface.
//
// Keys are "pkg.Type.Field"; values are function base keys ("pkg.Func",
// "pkg.Recv.Method", or a closure's "pkg.FuncLit:pos").
type funcFieldDispatch map[string][]string

// maxCompositeLitDepth bounds the composite-literal walk. Nesting this deep is
// pathological (a literal inside a literal inside a literal…), and the bound
// keeps a hand-written or generated monster from turning fact collection into
// the expensive part of the run.
const maxCompositeLitDepth = 12

// buildFuncFieldDispatch indexes every function value stored into a func-typed
// struct field. Three sources, because the same Go code is recorded three
// different ways depending on where the literal sits:
//
//   - File.StructInstances — the only record of a package-level
//     `var CmdWeb = &cli.Command{Action: runWeb}`, whose initializer is not
//     kept as a CallArgument anywhere. This is gitea's actual shape.
//   - assignment values — `app := &App{Commands: []*Command{{Action: runWeb}}}`,
//     where the nested literal survives in full in the CallArgument tree (it is
//     only the *rendered* StructInstance field that flattens it to types).
//   - call arguments — the literal passed straight into a call,
//     `app.Register(&Command{Action: runWeb})`.
//
// Every map iteration is sorted: this index feeds tree expansion, which decides
// which routes exist, so its order reaches the output (golden rule #1).
func buildFuncFieldDispatch(meta *metadata.Metadata) funcFieldDispatch {
	if meta == nil {
		return nil
	}
	d := funcFieldDispatch{}
	known := knownFunctionKeys(meta)

	for _, pkgName := range sortedMapKeys(meta.Packages) {
		pkg := meta.Packages[pkgName]
		if pkg == nil {
			continue
		}
		for _, fileName := range sortedMapKeys(pkg.Files) {
			file := pkg.Files[fileName]
			if file == nil {
				continue
			}
			d.addStructInstances(meta, known, pkgName, file)
			for _, fnName := range sortedMapKeys(file.Functions) {
				fn := file.Functions[fnName]
				if fn == nil {
					continue
				}
				for _, varName := range sortedMapKeys(fn.AssignmentMap) {
					for i := range fn.AssignmentMap[varName] {
						a := &fn.AssignmentMap[varName][i]
						d.walkCompositeLits(meta, known, pkgName, &a.Value, "", 0)
					}
				}
			}
		}
	}

	// Call arguments. The caller's package is the fallback context for an
	// unqualified function ident, since a literal written at a call site names
	// functions from the package it is written in.
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		ctxPkg := getString(meta, edge.Caller.Pkg)
		for _, arg := range edge.Args {
			d.walkCompositeLits(meta, known, ctxPkg, arg, "", 0)
		}
	}

	for key, vals := range d {
		sort.Strings(vals)
		d[key] = dedupeSortedStrings(vals)
	}
	return d
}

// keysFor returns the function base keys reachable through an edge that calls a
// func-typed struct field. An edge calling a real method (or anything else)
// yields nothing: only fields recorded in the index can match, and the index
// only holds func-typed fields.
func (d funcFieldDispatch) keysFor(meta *metadata.Metadata, edge *metadata.CallGraphEdge) []string {
	if len(d) == 0 || edge == nil || meta == nil {
		return nil
	}
	recv := strings.TrimPrefix(getString(meta, edge.Callee.RecvType), "*")
	if recv == "" {
		return nil
	}
	pkg := getString(meta, edge.Callee.Pkg)
	name := getString(meta, edge.Callee.Name)
	if pkg == "" || name == "" {
		return nil
	}
	// The receiver may arrive package-qualified ("pkg.Command"); the index is
	// keyed on the bare type name.
	if dot := strings.LastIndex(recv, "."); dot >= 0 {
		recv = recv[dot+1:]
	}
	return d[funcFieldKey(pkg, recv, name)]
}

func funcFieldKey(pkg, typeName, field string) string {
	return pkg + "." + typeName + "." + field
}

// addStructInstances records func-typed fields from the rendered struct
// instances of one file. StructInstance.Fields is field name -> rendered value,
// so this source only resolves a value that is a plain function name — which is
// exactly the `Action: runWeb` case it exists to cover. A nested literal renders
// as a type blob ("[]*Command{{string: web, func() error: func() error}}") and
// contributes nothing here; the assignment walk is what catches those.
func (d funcFieldDispatch) addStructInstances(meta *metadata.Metadata, known map[string]bool, pkgName string, file *metadata.File) {
	for i := range file.StructInstances {
		inst := &file.StructInstances[i]
		typeName := bareTypeName(getString(meta, inst.Type))
		fields := structFieldTypes(meta, pkgName, typeName)
		if len(fields) == 0 || len(inst.Fields) == 0 {
			continue
		}
		names := make([]string, 0, len(inst.Fields))
		byName := make(map[string]string, len(inst.Fields))
		for nameIdx, valIdx := range inst.Fields {
			name := getString(meta, nameIdx)
			if name == "" {
				continue
			}
			names = append(names, name)
			byName[name] = getString(meta, valIdx)
		}
		sort.Strings(names)
		for _, name := range names {
			if !isFuncTypeString(fields[name]) {
				continue
			}
			// A rendered value is only usable when it names a function; a
			// literal, a call or a nested literal renders to something that is
			// not a function key, and the existence check rejects it.
			if key := functionKeyFromName(known, pkgName, byName[name]); key != "" {
				k := funcFieldKey(pkgName, typeName, name)
				d[k] = append(d[k], key)
			}
		}
	}
}

// walkCompositeLits descends a CallArgument looking for composite literals and
// records each func-typed field initialised with a function value.
//
// expectType carries the type a literal cannot state for itself: the elements of
// `[]*Command{{…}}` are elided literals whose type comes from the slice, and a
// field's value literal takes its type from the field.
func (d funcFieldDispatch) walkCompositeLits(meta *metadata.Metadata, known map[string]bool, ctxPkg string, arg *metadata.CallArgument, expectType string, depth int) {
	if arg == nil || depth > maxCompositeLitDepth {
		return
	}
	if arg.GetKind() != metadata.KindCompositeLit {
		// Not a literal itself, but one may sit underneath: &T{…} is a unary,
		// f(T{…}) a call, and so on.
		for _, child := range []*metadata.CallArgument{arg.X, arg.Fun, arg.Sel} {
			d.walkCompositeLits(meta, known, ctxPkg, child, expectType, depth+1)
		}
		for _, child := range arg.Args {
			d.walkCompositeLits(meta, known, ctxPkg, child, "", depth+1)
		}
		return
	}

	typeName := compositeLitTypeName(arg)
	if typeName == "" {
		typeName = expectType
	}
	litPkg := compositeLitPkg(arg, ctxPkg)
	bare := bareTypeName(typeName)
	fields := structFieldTypes(meta, litPkg, bare)
	ordered := structFieldOrder(meta, litPkg, bare)
	elem := elementTypeName(typeName)

	for i, elt := range arg.Args {
		if elt == nil {
			continue
		}
		if elt.GetKind() == metadata.KindKeyValue {
			// A map literal's keys are values, not field names; the field
			// lookup below simply finds nothing for them.
			name := ""
			if elt.X != nil {
				name = elt.X.GetName()
			}
			fieldType := fields[name]
			if name != "" && isFuncTypeString(fieldType) {
				d.record(known, litPkg, bare, name, elt.Fun, ctxPkg)
			}
			next := fieldType
			if next == "" {
				next = elem
			}
			d.walkCompositeLits(meta, known, ctxPkg, elt.Fun, next, depth+1)
			continue
		}
		// Positional: a struct literal binds elements to fields in declaration
		// order (`Command{"web", runWeb}`); anything else (slice, array, map
		// value list) binds them to the element type.
		if i < len(ordered) && len(ordered) > 0 && elem == "" {
			name := ordered[i]
			if isFuncTypeString(fields[name]) {
				d.record(known, litPkg, bare, name, elt, ctxPkg)
			}
			d.walkCompositeLits(meta, known, ctxPkg, elt, fields[name], depth+1)
			continue
		}
		d.walkCompositeLits(meta, known, ctxPkg, elt, elem, depth+1)
	}
}

// record resolves a field's value to a function base key and indexes it.
func (d funcFieldDispatch) record(known map[string]bool, pkg, typeName, field string, value *metadata.CallArgument, ctxPkg string) {
	for _, key := range functionKeysOfValue(known, value, ctxPkg) {
		k := funcFieldKey(pkg, typeName, field)
		d[k] = append(d[k], key)
	}
}

// functionKeysOfValue resolves the value assigned to a func-typed field to the
// base key(s) of the function body it names.
//
// Honest by construction: only a value that *names* a function resolves. A
// variable holding a function ("Action: h.handler" where handler is a field, or
// a func-typed local) names no body, so it contributes nothing rather than a
// guess — the same posture as an unresolvable interface (golden rule #7).
func functionKeysOfValue(known map[string]bool, value *metadata.CallArgument, ctxPkg string) []string {
	if value == nil {
		return nil
	}
	switch value.GetKind() {
	case metadata.KindIdent:
		pkg := value.GetPkg()
		if pkg == "" {
			pkg = ctxPkg
		}
		if key := functionKeyFromName(known, pkg, value.GetName()); key != "" {
			return []string{key}
		}
	case metadata.KindFuncLit:
		// An inline `Action: func(c *cli.Context) error { … }` — the closure is
		// recorded as a function of its own, named by position.
		if name := value.GetName(); name != "" {
			if key := functionKeyFromName(known, ctxPkg, name); key != "" {
				return []string{key}
			}
		}
	case metadata.KindSelector:
		if value.Sel == nil {
			return nil
		}
		selName := value.Sel.GetName()
		if selName == "" {
			return nil
		}
		selPkg := value.Sel.GetPkg()
		if selPkg == "" {
			selPkg = ctxPkg
		}
		// Method value (`Action: srv.runWeb`): the receiver type completes the
		// key. Otherwise it is a cross-package function (`Action: web.RunWeb`).
		recv := ""
		if value.ReceiverType != nil {
			recv = value.ReceiverType.GetName()
		} else if value.X != nil && value.X.Type != -1 {
			recv = value.X.GetType()
		}
		recv = bareTypeName(recv)
		if recv != "" {
			if key := selPkg + "." + recv + "." + selName; known[key] {
				return []string{key}
			}
		}
		if key := functionKeyFromName(known, selPkg, selName); key != "" {
			return []string{key}
		}
	}
	return nil
}

// functionKeyFromName returns "pkg.name" when that names a recorded function.
// The name may already be package-qualified (a rendered StructInstance value),
// in which case it is trusted as written.
func functionKeyFromName(known map[string]bool, pkg, name string) string {
	if name == "" {
		return ""
	}
	if known[name] {
		return name
	}
	if pkg == "" {
		return ""
	}
	if key := pkg + "." + name; known[key] {
		return key
	}
	return ""
}

// knownFunctionKeys is the set of function base keys tree expansion can actually
// follow: every key that calls something (meta.Callers) or encloses a closure
// that does (meta.ParentFunctions). Requiring membership is what keeps a field
// whose value is a variable, a call or a literal out of the index — and it is
// deliberately the *expandable* set rather than the declared one, since a
// function that calls nothing contributes no routes either way.
//
// Closures qualify: an inline `Action: func() error { … }` is recorded as a
// caller in its own right, named by position, which is how the common urfave/cli
// form resolves at all.
func knownFunctionKeys(meta *metadata.Metadata) map[string]bool {
	known := make(map[string]bool, len(meta.Callers)+len(meta.ParentFunctions))
	for key := range meta.Callers {
		known[key] = true
	}
	for key := range meta.ParentFunctions {
		known[key] = true
	}
	return known
}

// structFieldTypes maps a struct's field names to their type strings.
func structFieldTypes(meta *metadata.Metadata, pkg, typeName string) map[string]string {
	if pkg == "" || typeName == "" {
		return nil
	}
	p, ok := meta.Packages[pkg]
	if !ok || p == nil {
		return nil
	}
	for _, fileName := range sortedMapKeys(p.Files) {
		file := p.Files[fileName]
		if file == nil {
			continue
		}
		typ, ok := file.Types[typeName]
		if !ok || typ == nil || len(typ.Fields) == 0 {
			continue
		}
		out := make(map[string]string, len(typ.Fields))
		for i := range typ.Fields {
			out[getString(meta, typ.Fields[i].Name)] = getString(meta, typ.Fields[i].Type)
		}
		return out
	}
	return nil
}

// structFieldOrder lists a struct's field names in declaration order, for
// positional literals.
func structFieldOrder(meta *metadata.Metadata, pkg, typeName string) []string {
	if pkg == "" || typeName == "" {
		return nil
	}
	p, ok := meta.Packages[pkg]
	if !ok || p == nil {
		return nil
	}
	for _, fileName := range sortedMapKeys(p.Files) {
		file := p.Files[fileName]
		if file == nil {
			continue
		}
		typ, ok := file.Types[typeName]
		if !ok || typ == nil || len(typ.Fields) == 0 {
			continue
		}
		out := make([]string, 0, len(typ.Fields))
		for i := range typ.Fields {
			out = append(out, getString(meta, typ.Fields[i].Name))
		}
		return out
	}
	return nil
}

// isFuncTypeString reports whether a recorded field type is a function type.
// Metadata renders a func field's type as the bare word "func" in some paths and
// as a full signature in others, so both forms count.
func isFuncTypeString(t string) bool {
	t = strings.TrimPrefix(t, "*")
	return t == "func" || strings.HasPrefix(t, "func(")
}

// compositeLitTypeName renders the type a composite literal states for itself.
// An elided literal (`{Name: "web"}` inside a slice) states none.
func compositeLitTypeName(arg *metadata.CallArgument) string {
	if arg == nil || arg.X == nil {
		return ""
	}
	return typeExprName(arg.X)
}

// typeExprName renders a type expression argument back to a type string, far
// enough to recognise slices, pointers and named types.
func typeExprName(arg *metadata.CallArgument) string {
	if arg == nil {
		return ""
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		return arg.GetName()
	case metadata.KindStar:
		if inner := typeExprName(arg.X); inner != "" {
			return "*" + inner
		}
	case metadata.KindArrayType, metadata.KindSlice:
		if inner := typeExprName(arg.X); inner != "" {
			return "[]" + inner
		}
	case metadata.KindSelector:
		if arg.Sel != nil {
			return arg.Sel.GetName()
		}
	}
	return ""
}

// compositeLitPkg returns the package that owns a literal's type, falling back
// to the context package for an unqualified or elided type.
func compositeLitPkg(arg *metadata.CallArgument, ctxPkg string) string {
	if arg != nil && arg.X != nil {
		for probe := arg.X; probe != nil; probe = probe.X {
			if pkg := probe.GetPkg(); pkg != "" {
				return pkg
			}
			if probe.Sel != nil {
				if pkg := probe.Sel.GetPkg(); pkg != "" {
					return pkg
				}
			}
		}
	}
	return ctxPkg
}

// bareTypeName strips pointer, slice and package qualification down to the
// declared type name the Types map is keyed by.
func bareTypeName(t string) string {
	t = strings.TrimSpace(t)
	t = strings.TrimPrefix(t, "[]")
	t = strings.TrimPrefix(t, "*")
	t = strings.TrimPrefix(t, "[]")
	t = strings.TrimPrefix(t, "*")
	if dot := strings.LastIndex(t, "."); dot >= 0 {
		t = t[dot+1:]
	}
	return t
}

// elementTypeName returns the element type of a slice, array or map type, or ""
// for anything else.
func elementTypeName(t string) string {
	switch {
	case strings.HasPrefix(t, "[]"):
		return t[2:]
	case strings.HasPrefix(t, "map["):
		if close := strings.Index(t, "]"); close > 0 {
			return t[close+1:]
		}
	case strings.HasPrefix(t, "["):
		// Fixed-size array: [4]*Command.
		if close := strings.Index(t, "]"); close > 0 {
			return t[close+1:]
		}
	}
	return ""
}

func dedupeSortedStrings(in []string) []string {
	out := in[:0]
	var prev string
	for i, s := range in {
		if i > 0 && s == prev {
			continue
		}
		prev = s
		out = append(out, s)
	}
	return out
}

// sortedMapKeys returns a map's keys in sorted order — determinism is a feature
// (golden rule #1), and this index decides which routes exist.
func sortedMapKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
