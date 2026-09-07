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
	"log"
	"maps"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/ehabterra/apispec/internal/typemodel"
)

// applyNaming rewrites component names and operationIds according to
// cfg.Naming, after the spec is otherwise complete.
//
// It runs as a post-pass rather than at each producer because a component name
// is written in many places — the components map, and a $ref in a schema, a
// property, an allOf member, a parameter, a request body, a response, a
// discriminator mapping. Renaming at the source would mean threading the
// decision through all of them and would dangle a ref the day one is missed;
// renaming afterwards, from one table, either rewrites every occurrence or
// none. The fixture asserts no dangling $ref for exactly this reason.
//
// usedTypes carries the Go type names the components were built from, which is
// what makes a short name derivable at all: a component KEY has already been
// sanitized ("pkg_Type"), and splitting that back apart would be guesswork on
// any type or package containing an underscore.
func applyNaming(spec *OpenAPISpec, cfg *APISpecConfig, usedTypes map[string]*Schema) {
	if spec == nil || cfg == nil {
		return
	}
	switch cfg.Naming.SchemaNames {
	case "", NamingFull:
	case NamingShort:
		applySchemaRenames(spec, shortSchemaNames(spec.Components, usedTypes))
	default:
		log.Printf("[naming] unknown schemaNames %q, keeping %q", cfg.Naming.SchemaNames, NamingFull)
	}
	switch cfg.Naming.OperationID {
	case "", NamingFull:
		repairOperationIDs(spec)
	case NamingReceiverMethod, NamingMethodPath:
		applyOperationIDs(spec, cfg.Naming.OperationID)
	default:
		log.Printf("[naming] unknown operationId %q, keeping %q", cfg.Naming.OperationID, NamingFull)
		repairOperationIDs(spec)
	}
}

// repairOperationIDs replaces the operationIds that cannot serve as one, and
// leaves every other id exactly as it was.
//
// An operationId must be unique across the document — a client generator keys
// its method names on it — and the fully-qualified handler symbol is not, for
// a reason no amount of resolution fixes: a shared middleware or wrapper IS the
// resolved handler for many routes. On gitea that put 1109 operations under 700
// ids, `reqToken` alone accounting for 46 (issue #459).
//
// Two kinds of id are replaced:
//
//   - one claimed by more than one operation. ALL of its holders are replaced,
//     not the runners-up: the symbol has been shown not to identify an
//     operation, so keeping it for whichever route sorted first would be
//     arbitrary in a way a reader cannot see.
//   - one that is not an identifier at all. `invalid type` is go/types
//     rendering Typ[Invalid] — a handler whose type did not check — and it
//     reached 153 operations on gitea. It says nothing and cannot be dropped
//     into generated code.
//
// The replacement is the operation's own method-and-path identity, which is
// derived from the document rather than from the code, so it exists for every
// operation and describes the one thing that is unique about it. Assignment is
// in sorted (path, method) order and skips ids already spoken for, so the
// result is a pure function of the document (golden rule #1).
func repairOperationIDs(spec *OpenAPISpec) {
	ops := sortedOperations(spec)
	if len(ops) == 0 {
		return
	}

	counts := map[string]int{}
	for _, o := range ops {
		counts[o.op.OperationID]++
	}

	// Ids that stay are spoken for, so a replacement never lands on one.
	taken := map[string]bool{}
	for _, o := range ops {
		if id := o.op.OperationID; counts[id] == 1 && usableOperationID(id) {
			taken[id] = true
		}
	}

	for _, o := range ops {
		id := o.op.OperationID
		if counts[id] == 1 && usableOperationID(id) {
			continue
		}
		for _, want := range operationIDCandidates(NamingMethodPath, o.method, o.path, id, len(ops)) {
			if want == "" || taken[want] {
				continue
			}
			taken[want] = true
			o.op.OperationID = want
			break
		}
	}
}

// usableOperationID reports whether an id can serve as an operationId at all.
//
// "invalid type" is go/types' rendering of Typ[Invalid], which reaches the
// handler identity when a package does not type-check; it is prose, not a
// name. An empty id is no better.
func usableOperationID(id string) bool {
	return id != "" && !strings.Contains(id, invalidTypeRendering)
}

// invalidTypeRendering is what go/types prints for Typ[Invalid]. Matched as a
// string because that is the only form it arrives in here — the spec layer sees
// a rendered name, not a types.Type.
const invalidTypeRendering = "invalid type"

// shortEntry is one component considered for shortening: the key it has now,
// the import path of its Go type, and the unqualified rendering of that type.
type shortEntry struct {
	key  string
	pkg  string
	bare string
}

// shortSchemaNames maps each component key to its unqualified type name,
// qualifying only what would otherwise collide.
//
// Two packages with a `Components` type are ordinary, and letting one of them
// win the bare name would be worse than the long names — the winner would
// depend on nothing a reader could see. So a colliding group is qualified as a
// group: every member takes the shortest suffix of its package path that tells
// all of them apart, extended one segment at a time. Groups are visited in
// sorted order, and a name already claimed is never taken, so the result is a
// pure function of the input (golden rule #1).
func shortSchemaNames(components *Components, usedTypes map[string]*Schema) map[string]string {
	if components == nil || len(components.Schemas) == 0 {
		return nil
	}
	var entries []shortEntry
	for _, goType := range slices.Sorted(maps.Keys(usedTypes)) {
		key := schemaComponentNameReplacer.Replace(goType)
		if _, ok := components.Schemas[key]; !ok {
			continue
		}
		ref := typemodel.Parse(goType)
		if ref == nil || ref.Kind != typemodel.KindNamed {
			// A wrapper-typed key (*T, []T) is an artifact rather than a type
			// anyone references — gitea has one such orphan component, from a
			// pointer that was never stripped. Shortening it would render as
			// "*T", sanitize to a leading underscore, and collide with the
			// component for T. Left alone, so the artifact stays visible and
			// this pass invents nothing.
			continue
		}
		core := ref.Core()
		if core == nil || core.Name == "" || core.Pkg == "" {
			continue // unqualified or opaque: nothing to shorten
		}
		bare := ref.Simple()
		if bare == "" {
			continue
		}
		entries = append(entries, shortEntry{key: key, pkg: core.Pkg, bare: bare})
	}
	if len(entries) == 0 {
		return nil
	}

	byBare := map[string][]shortEntry{}
	for _, e := range entries {
		byBare[e.bare] = append(byBare[e.bare], e)
	}

	renames := map[string]string{}
	taken := map[string]string{} // proposed name -> component key that claimed it
	// A component this pass skips — wrapper-typed, unqualified, opaque — keeps
	// the key it has. Reserve those first: a short name that happened to equal
	// one would put two schemas under a single key, and the map would keep only
	// the last, silently redirecting the other's $refs to the wrong type.
	shortened := make(map[string]bool, len(entries))
	for _, e := range entries {
		shortened[e.key] = true
	}
	for key := range components.Schemas {
		if !shortened[key] {
			taken[key] = key
		}
	}
	for _, bare := range slices.Sorted(maps.Keys(byBare)) {
		group := byBare[bare]
		level := 0
		if len(group) > 1 {
			level = distinguishingLevel(group)
		}
		for _, e := range group {
			// A cross-group clash (a qualified name meeting an unqualified one)
			// escalates this entry alone until it is free, and never past the
			// full path, where uniqueness is guaranteed by construction.
			var name string
			for lvl := level; ; lvl++ {
				name = schemaComponentNameReplacer.Replace(qualifyName(e.bare, e.pkg, lvl))
				if claimed, clash := taken[name]; !clash || claimed == e.key {
					break
				}
				if lvl > strings.Count(e.pkg, "/")+1 {
					name = e.key // give up: keep the fully-qualified key
					break
				}
			}
			taken[name] = e.key
			if name != e.key {
				renames[e.key] = name
			}
		}
	}
	return renames
}

// distinguishingLevel is the smallest number of trailing package-path segments
// that tells every member of a colliding group apart, or the longest path
// length when no level does (two types of the same name in one package cannot
// happen, so this is a safety net rather than an expected outcome).
func distinguishingLevel(group []shortEntry) int {
	max := 0
	for _, e := range group {
		if n := strings.Count(e.pkg, "/") + 1; n > max {
			max = n
		}
	}
	for level := 1; level <= max; level++ {
		seen := map[string]bool{}
		unique := true
		for _, e := range group {
			q := qualifyName(e.bare, e.pkg, level)
			if seen[q] {
				unique = false
				break
			}
			seen[q] = true
		}
		if unique {
			return level
		}
	}
	return max
}

// qualifyName prefixes bare with the last `level` segments of pkg. Level 0 is
// the unqualified name.
func qualifyName(bare, pkg string, level int) string {
	if level <= 0 {
		return bare
	}
	segs := strings.Split(pkg, "/")
	if level > len(segs) {
		level = len(segs)
	}
	return strings.Join(segs[len(segs)-level:], "_") + "_" + bare
}

// applySchemaRenames renames the component keys and every $ref that names one.
//
// The rewrite goes through mapSchemaRefs — the same traversal the dangling-ref
// check uses — so a ref site cannot be renamed by one and missed by the other.
func applySchemaRenames(spec *OpenAPISpec, renames map[string]string) {
	if len(renames) == 0 || spec == nil || spec.Components == nil {
		return
	}
	refFor := make(map[string]string, len(renames))
	for from, to := range renames {
		refFor[refComponentsSchemasPrefix+from] = refComponentsSchemasPrefix + to
	}

	renamed := make(map[string]*Schema, len(spec.Components.Schemas))
	for key, s := range spec.Components.Schemas {
		if to, ok := renames[key]; ok {
			key = to
		}
		renamed[key] = s
	}
	spec.Components.Schemas = renamed

	mapSchemaRefs(spec, func(ref string) string {
		if to, ok := refFor[ref]; ok {
			return to
		}
		return ref
	})
}

// applyOperationIDs rewrites every operationId in the chosen style, and
// guarantees the result is unique.
//
// Uniqueness is not a nicety here. The fully-qualified default is NOT unique on
// a real project: gitea's spec carries 1109 operations under 700 distinct ids,
// because a shared middleware or wrapper is the resolved handler for many
// routes (issue #459). Inheriting that would hand a caller a document most
// generators reject.
//
// So a clash escalates rather than surrendering: receiver-method falls back to
// the operation's own method-and-path identity, and a clash THERE — two paths
// normalizing to one identifier, since /a-b and /a/b both read as "getAB" —
// takes a numeric suffix. The pair is unique in OpenAPI; the identifier derived
// from it is not, which is why the numbering exists rather than being
// unreachable belt-and-braces. Operations are visited in sorted (path, method) order, so which one
// takes the plain name is decided by the document, not by map order.
func applyOperationIDs(spec *OpenAPISpec, style string) {
	ops := sortedOperations(spec)
	taken := map[string]bool{}
	for _, o := range ops {
		for _, want := range operationIDCandidates(style, o.method, o.path, o.op.OperationID, len(ops)) {
			if want == "" || taken[want] {
				continue
			}
			taken[want] = true
			o.op.OperationID = want
			break
		}
	}
}

// operationIDCandidates lists the ids to try for one operation, best first: the
// chosen style, then the method-and-path identity, then that identity numbered.
//
// The numbering is not decoration. A method and path pair is unique in OpenAPI,
// but the IDENTIFIER derived from it is not: methodPathID drops every
// non-alphanumeric character, so /a-b, /a/b and /aB all read as "getAB". The
// suffix is bounded by the number of operations, so a name is always available
// — with n operations, at most n-1 can be ahead of this one on any given name.
func operationIDCandidates(style, method, path, current string, operations int) []string {
	byPath := methodPathID(method, path)
	var out []string
	switch style {
	case NamingMethodPath:
		out = []string{byPath}
	case NamingReceiverMethod:
		out = []string{receiverMethodID(current), byPath}
	}
	for i := 2; i <= operations+1; i++ {
		out = append(out, byPath+strconv.Itoa(i))
	}
	return out
}

// receiverMethodID drops the import path from a fully-qualified operationId,
// keeping the receiver and method that identify the handler in its own package
// ("…/internal/httpapi.estimateHandler.updateLine" -> "estimateHandler.updateLine").
//
// A generic handler is qualified INSIDE its type argument as well
// ("…/modules/web.Bind[…/services/forms.InstallForm]"), so the base and the
// argument list are unqualified separately. Trimming the whole string at its
// last "/" would cut inside the brackets and leave "InstallForm]" — which is
// how gitea's spec looked before this was split out.
//
// A closure has no receiver and its tail is a source position
// ("pkg.FuncLit:router.go:998:21"); dropping the path is still an improvement,
// and method-path is the better choice for a codebase written that way.
func receiverMethodID(full string) string {
	if full == "" {
		return ""
	}
	base, args := full, ""
	if i := strings.Index(full, "["); i >= 0 {
		base, args = full[:i], full[i:]
	}
	base = unqualifySymbol(base)
	if args == "" {
		return base
	}
	// Each type argument is unqualified the same way, so Bind[…forms.X] reads
	// as Bind[X] rather than carrying a second import path.
	inner := strings.TrimSuffix(strings.TrimPrefix(args, "["), "]")
	parts := strings.Split(inner, ",")
	for i, p := range parts {
		parts[i] = unqualifySymbol(strings.TrimSpace(p))
	}
	return base + "[" + strings.Join(parts, ", ") + "]"
}

// unqualifySymbol drops an import path and the package name from a qualified Go
// symbol, keeping everything that identifies it within its own package.
func unqualifySymbol(sym string) string {
	if i := strings.LastIndex(sym, "/"); i >= 0 {
		sym = sym[i+1:]
	}
	// What remains is "pkgname.Rest"; drop the package name.
	if i := strings.Index(sym, "."); i >= 0 && i+1 < len(sym) {
		sym = sym[i+1:]
	}
	return sym
}

// methodPathID builds "getUsersByIdItems" from GET /users/{id}/items: the verb,
// then each segment capitalized, with a path parameter reading as "By<Name>".
func methodPathID(method, path string) string {
	var b strings.Builder
	b.WriteString(strings.ToLower(method))
	for _, seg := range strings.Split(path, "/") {
		if seg == "" {
			continue
		}
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			b.WriteString("By")
			b.WriteString(pathWord(strings.Trim(seg, "{}")))
			continue
		}
		b.WriteString(pathWord(seg))
	}
	return b.String()
}

// pathWord renders one path segment as an identifier fragment: word boundaries
// at every non-alphanumeric character, each word capitalized. Interior case is
// preserved, so a camelCase segment survives — unlike security_lookup's
// camelSegment, which lowercases what follows and would read "estimateLine" as
// "Estimateline".
func pathWord(s string) string {
	var b strings.Builder
	upper := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upper {
				b.WriteRune(unicode.ToUpper(r))
				upper = false
			} else {
				b.WriteRune(r)
			}
		default:
			upper = true
		}
	}
	return b.String()
}

// operationsByMethod indexes a path item's operations by uppercase HTTP method,
// so a pass can visit them in a sorted, reproducible order.
func operationsByMethod(item *PathItem) map[string]*Operation {
	out := map[string]*Operation{}
	for method, op := range map[string]*Operation{
		"GET": item.Get, "POST": item.Post, "PUT": item.Put, "DELETE": item.Delete,
		"PATCH": item.Patch, "OPTIONS": item.Options, "HEAD": item.Head,
	} {
		if op != nil {
			out[method] = op
		}
	}
	return out
}

// opRef is one operation with the path and method that identify it.
type opRef struct {
	path, method string
	op           *Operation
}

// sortedOperations lists every operation in (path, method) order, so a pass
// that assigns names cannot depend on map order (golden rule #1).
func sortedOperations(spec *OpenAPISpec) []opRef {
	if spec == nil {
		return nil
	}
	var ops []opRef
	for _, path := range slices.Sorted(maps.Keys(spec.Paths)) {
		item := spec.Paths[path]
		byMethod := operationsByMethod(&item)
		for _, method := range slices.Sorted(maps.Keys(byMethod)) {
			if op := byMethod[method]; op != nil {
				ops = append(ops, opRef{path: path, method: method, op: op})
			}
		}
		spec.Paths[path] = item
	}
	return ops
}
