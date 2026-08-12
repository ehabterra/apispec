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
)

// UnresolvedRef is a $ref the finished document could not satisfy, after it has
// been repaired.
type UnresolvedRef struct {
	// Component is the name the $ref pointed at, e.g.
	// "github_com_golang-jwt_jwt_v5_RegisteredClaims".
	Component string
	// GoType is the type that name was derived from, when it can be recovered
	// — "github.com/golang-jwt/jwt/v5.RegisteredClaims". This is the actionable
	// half: it names the dependency to register under `externalTypes`, which
	// the mangled component name does not. Empty when the type is unknown.
	GoType string
	// Sites is how many references pointed at the missing component.
	Sites int
}

// repairDanglingRefs makes the assembled document internally consistent: every
// $ref it contains resolves to a component that exists.
//
// This is a last line of defence, not a substitute for resolving types
// properly. Four separate causes of dangling references have been fixed
// (#325, #326, #329, #333) and each was found the same way — by loading the
// output into a viewer, which is not a check anyone runs on their own project.
// The generator tests assert this per fixture (noDanglingRefs); nothing
// asserted it for a user's actual code, which is where the unregistered
// dependency types live.
//
// A missing target is REPAIRED rather than dropped: the reference is left in
// place and the honest placeholder registered under it, so the document stays
// loadable while the report says what was substituted. Removing the reference
// instead would silently change an operation's shape.
func repairDanglingRefs(spec *OpenAPISpec, usedTypes map[string]*Schema) []UnresolvedRef {
	if spec == nil {
		return nil
	}

	counts := map[string]int{}
	forEachSchemaRef(spec, func(name string) { counts[name]++ })
	if len(counts) == 0 {
		return nil
	}

	defined := map[string]bool{}
	if spec.Components != nil {
		for name := range spec.Components.Schemas {
			defined[name] = true
		}
	}

	missing := make([]string, 0, len(counts))
	for name := range counts {
		if !defined[name] {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	// Sorted: the repair adds components, and map order would decide their
	// insertion order and the report's order (golden rule #1).
	sort.Strings(missing)

	goTypes := goTypeByComponentName(usedTypes)

	if spec.Components == nil {
		spec.Components = &Components{}
	}
	if spec.Components.Schemas == nil {
		spec.Components.Schemas = map[string]*Schema{}
	}

	out := make([]UnresolvedRef, 0, len(missing))
	for _, name := range missing {
		goType := goTypes[name]
		described := goType
		if described == "" {
			described = name
		}
		spec.Components.Schemas[name] = unresolvedExternalPlaceholder(described)
		out = append(out, UnresolvedRef{Component: name, GoType: goType, Sites: counts[name]})
	}
	return out
}

// goTypeByComponentName inverts the component naming, so a report can name the
// Go type rather than the mangled key. The mangling is lossy — "/" and "." and
// the type separator all become "_" — so it cannot be reversed by rewriting the
// string; it is recovered by mangling the names we know and keeping the map.
func goTypeByComponentName(usedTypes map[string]*Schema) map[string]string {
	if len(usedTypes) == 0 {
		return nil
	}
	out := make(map[string]string, len(usedTypes))
	for goType := range usedTypes {
		name := schemaComponentNameReplacer.Replace(strings.TrimPrefix(goType, "*"))
		// Several Go types can mangle to one name; prefer the shortest, which
		// is the least decorated spelling of the same thing.
		if prev, ok := out[name]; !ok || len(goType) < len(prev) {
			out[name] = goType
		}
	}
	return out
}

// forEachSchemaRef visits every component-schema reference in the document.
//
// Every location matters: a reference missed here is one the repair leaves
// dangling, which is the whole failure this function exists to prevent. Hence
// walking the typed document rather than the schemas map alone — request
// bodies, responses, headers, parameters and the components section each carry
// their own.
func forEachSchemaRef(spec *OpenAPISpec, visit func(name string)) {
	seen := map[*Schema]bool{}

	var walk func(s *Schema)
	walk = func(s *Schema) {
		if s == nil || seen[s] {
			return
		}
		seen[s] = true

		if name, ok := componentSchemaName(s.Ref); ok {
			visit(name)
		}
		for _, p := range s.Properties {
			walk(p)
		}
		walk(s.Items)
		walk(s.AdditionalProperties)
		walk(s.Not)
		for _, list := range [][]*Schema{s.AllOf, s.OneOf, s.AnyOf} {
			for _, member := range list {
				walk(member)
			}
		}
	}

	walkParams := func(params []Parameter) {
		for _, p := range params {
			if name, ok := componentSchemaName(p.Ref); ok {
				visit(name)
			}
			walk(p.Schema)
		}
	}
	walkContent := func(content map[string]MediaType) {
		for _, mt := range content {
			walk(mt.Schema)
		}
	}
	walkResponses := func(responses map[string]Response) {
		for _, r := range responses {
			walkContent(r.Content)
			for _, h := range r.Headers {
				walk(h.Schema)
			}
		}
	}

	for _, item := range spec.Paths {
		walkParams(item.Parameters)
		for _, op := range []*Operation{item.Get, item.Post, item.Put, item.Delete, item.Patch, item.Head, item.Options} {
			if op == nil {
				continue
			}
			walkParams(op.Parameters)
			if op.RequestBody != nil {
				walkContent(op.RequestBody.Content)
			}
			walkResponses(op.Responses)
		}
	}

	if spec.Components == nil {
		return
	}
	for _, s := range spec.Components.Schemas {
		walk(s)
	}
	for _, p := range spec.Components.Parameters {
		if p == nil {
			continue
		}
		walk(p.Schema)
	}
	for _, rb := range spec.Components.RequestBodies {
		if rb == nil {
			continue
		}
		walkContent(rb.Content)
	}
	for _, r := range spec.Components.Responses {
		if r == nil {
			continue
		}
		walkContent(r.Content)
		for _, h := range r.Headers {
			walk(h.Schema)
		}
	}
	for _, h := range spec.Components.Headers {
		if h == nil {
			continue
		}
		walk(h.Schema)
	}
}

// componentSchemaName returns the component name a $ref points at, and whether
// it is a local components/schemas reference at all. A remote or non-schema
// reference is not ours to satisfy.
func componentSchemaName(ref string) (string, bool) {
	if ref == "" || !strings.HasPrefix(ref, refComponentsSchemasPrefix) {
		return "", false
	}
	name := strings.TrimPrefix(ref, refComponentsSchemasPrefix)
	if name == "" {
		return "", false
	}
	return name, true
}
