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
	"go/ast"
	"maps"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// effectiveField is one field of a struct as the WIRE sees it: the field
// itself, the type that declares it, and how deep the embedding was.
type effectiveField struct {
	field metadata.Field
	// owner is the type that declares the field, which is where its own
	// package context comes from — a promoted field's type names resolve
	// against the embedded type's package, not the embedding one.
	owner *metadata.Type
	depth int
	// tagged is whether the field carries an explicit json name, which decides
	// collisions at equal depth.
	tagged bool
}

// effectiveJSONFields returns the fields a struct actually serialises, with
// embedded types resolved the way encoding/json resolves them.
//
// A struct that embeds another documented NONE of the embedded fields: the
// mapper walked Type.Fields, and an embedded type is recorded in Type.Embeds
// instead. `type Item struct { Base; Extra string }` published only `Extra`,
// while the wire carried Base's fields too — so any response whose type embeds
// a shared Base, Meta or Envelope was under-documented by exactly what the
// embed carries, silently (issue #487).
//
// The rules are encoding/json's, not an approximation of them:
//
//   - An UNTAGGED embed promotes the embedded type's fields into this one, at a
//     depth one greater.
//   - A TAGGED embed does not promote: it is an ordinary field named by the tag,
//     whose value is the embedded object. This is why metadata had to start
//     recording the tag — without it the two forms are identical.
//   - An embedded NON-STRUCT contributes one field named for its type.
//   - Where several fields share a JSON name, the SHALLOWEST wins; at equal
//     depth exactly one tagged field wins, and otherwise they all drop out —
//     Go's rule, and the reason `Shadow{Base; Kind string `json:"kind"`}`
//     publishes the outer Kind and not Base's.
func effectiveJSONFields(meta *metadata.Metadata, typ *metadata.Type) []effectiveField {
	if meta == nil || typ == nil {
		return nil
	}
	var all []effectiveField
	collectJSONFields(meta, typ, 0, map[*metadata.Type]bool{}, &all)
	return resolveFieldCollisions(meta, all)
}

// collectJSONFields gathers every candidate field breadth-first by depth,
// before any collision is resolved: Go's rule compares candidates across the
// whole tree, so none can be discarded while walking it.
//
// There is no depth cap. A chain of seven untagged embeds is legal Go whose
// leaf fields encoding/json promotes, and a cap would have silently dropped
// them; termination comes from the per-path visited set below instead
// (CodeRabbit on #488).
func collectJSONFields(meta *metadata.Metadata, typ *metadata.Type, depth int, visited map[*metadata.Type]bool, out *[]effectiveField) {
	if typ == nil || visited[typ] {
		return
	}
	visited[typ] = true

	// Walked in DECLARATION order, interleaving the two lists metadata keeps
	// separately: before the i-th named field, every embed recorded as sitting
	// at position i. Field order is part of what a struct says — an embed
	// declared first promotes its fields ahead of the outer ones — and reading
	// the lists one after the other put every promoted field last regardless
	// (Type.EmbedAt, issue #487).
	next := 0
	emitEmbedsBefore := func(pos int) {
		for ; next < len(typ.Embeds); next++ {
			if embedAt(typ, next) > pos {
				return
			}
			collectEmbed(meta, typ, next, depth, visited, out)
		}
	}
	for i, f := range typ.Fields {
		emitEmbedsBefore(i)
		*out = append(*out, effectiveField{
			field:  f,
			owner:  typ,
			depth:  depth,
			tagged: extractJSONName(getStringFromPool(meta, f.Tag)) != "",
		})
	}
	emitEmbedsBefore(len(typ.Fields))
}

// embedAt is the position the i-th embed was declared at, or the end when the
// metadata predates EmbedAt — which keeps a deserialised older metadata.yaml
// working, with the embeds last rather than not at all.
func embedAt(typ *metadata.Type, i int) int {
	if i < len(typ.EmbedAt) {
		return typ.EmbedAt[i]
	}
	return len(typ.Fields)
}

// collectEmbed resolves one embedded field into the candidates it contributes.
func collectEmbed(meta *metadata.Metadata, typ *metadata.Type, i, depth int, visited map[*metadata.Type]bool, out *[]effectiveField) {
	embedIdx := typ.Embeds[i]
	name := getStringFromPool(meta, embedIdx)
	if name == "" {
		return
	}
	tag := ""
	if i < len(typ.EmbedTags) {
		tag = getStringFromPool(meta, typ.EmbedTags[i])
	}
	// `json:"-"` on an embed drops it whole, promotion and all.
	if jsonFieldOmitted(tag) {
		return
	}
	if jsonName := extractJSONName(tag); jsonName != "" {
		// Tagged: an ordinary field carrying the embedded object, not a
		// promotion. Synthesised as a field so the rest of the pipeline —
		// schema mapping, collision resolution — treats it like any other.
		*out = append(*out, effectiveField{
			field: metadata.Field{
				Name: embedIdx,
				Type: embedIdx,
				Tag:  typ.EmbedTags[i],
			},
			owner:  typ,
			depth:  depth,
			tagged: true,
		})
		return
	}

	embedded := embeddedType(meta, name, getStringFromPool(meta, typ.Pkg))
	if embedded == nil {
		// The declaration is not visible — an external type, most often.
		// encoding/json promotes whatever exported fields it has, and we cannot
		// see them, so this embed contributes NOTHING. Inventing a field named
		// for the type would be wrong for the common case: a `url.Userinfo`
		// embed publishes no properties at all, because every field it has is
		// unexported (golden rule #7).
		return
	}
	if !isStructType(meta, embedded) {
		// An embedded NON-STRUCT — `type ID string` — is one field named for the
		// type. Only claimed when the declaration is visible enough to show it
		// is not a struct; otherwise the branch above applies.
		*out = append(*out, effectiveField{
			field:  metadata.Field{Name: meta.StringPool.Get(embeddedFieldName(name)), Type: embedIdx},
			owner:  typ,
			depth:  depth,
			tagged: false,
		})
		return
	}
	// A per-PATH guard, cloned rather than shared. Two embeds reaching the same
	// type are two candidates at equal depth — which is exactly the ambiguity
	// encoding/json resolves by sending NEITHER — and a single set collapsed
	// them into one, publishing a property the encoder drops: `Outer{A; B}`
	// where both embed Base emitted Base's fields, where Go emits none of them
	// (CodeRabbit on #488).
	//
	// It still terminates without a depth cap: each step adds a type to this
	// path's set and never revisits it, and the set of declared types is finite.
	// Go forbids recursive embedding outright, so the guard is for metadata that
	// could not describe a compiling program.
	collectJSONFields(meta, embedded, depth+1, maps.Clone(visited), out)
}

// resolveFieldCollisions applies Go's shallowest-wins rule to candidates that
// share a JSON name.
func resolveFieldCollisions(meta *metadata.Metadata, all []effectiveField) []effectiveField {
	byName := map[string][]effectiveField{}
	var order []string
	for _, ef := range all {
		name := jsonFieldName(meta, ef.field)
		if name == "" {
			continue
		}
		if _, seen := byName[name]; !seen {
			order = append(order, name)
		}
		byName[name] = append(byName[name], ef)
	}

	out := make([]effectiveField, 0, len(order))
	for _, name := range order {
		if winner, ok := shallowestField(byName[name]); ok {
			out = append(out, winner)
		}
	}
	// DECLARATION order, which is what `order` records: the fields of the type
	// itself first, then each embed's in turn. Already deterministic — metadata
	// keeps both lists in source order — and it is what the `required` array
	// reads as, so sorting by name here would churn every existing schema for
	// nothing (golden rule #1 is about order that VARIES, not about sorting for
	// its own sake).
	return out
}

// shallowestField picks the candidate encoding/json would serialise, or reports
// that the name is dropped because the choice is genuinely ambiguous.
func shallowestField(candidates []effectiveField) (effectiveField, bool) {
	if len(candidates) == 0 {
		return effectiveField{}, false
	}
	best := candidates[0].depth
	for _, c := range candidates[1:] {
		if c.depth < best {
			best = c.depth
		}
	}
	var shallow []effectiveField
	for _, c := range candidates {
		if c.depth == best {
			shallow = append(shallow, c)
		}
	}
	if len(shallow) == 1 {
		return shallow[0], true
	}
	// Equal depth: exactly one tagged field wins; otherwise Go drops them all
	// rather than choosing, and so do we (golden rule #7).
	var tagged []effectiveField
	for _, c := range shallow {
		if c.tagged {
			tagged = append(tagged, c)
		}
	}
	if len(tagged) == 1 {
		return tagged[0], true
	}
	return effectiveField{}, false
}

// jsonFieldName is the name a field appears under on the wire, or "" when it
// never appears — unexported, or `json:"-"`.
func jsonFieldName(meta *metadata.Metadata, f metadata.Field) string {
	name := getStringFromPool(meta, f.Name)
	tag := getStringFromPool(meta, f.Tag)
	if jsonFieldOmitted(tag) {
		return ""
	}
	if jsonName := extractJSONName(tag); jsonName != "" {
		return jsonName
	}
	// The blank marker carries struct-level validation, not a field.
	if name == "_" || !ast.IsExported(name) {
		return ""
	}
	return name
}

// embeddedType resolves an embedded type name to its declaration, following the
// pointer form (`*Meta` embeds exactly as `Meta` does).
//
// An embed is usually written unqualified — `Base`, not `pkg.Base` — so the
// embedding type's own package is where to look when the name carries none.
// Without that every same-package embed resolved to nothing and was documented
// as a field literally named "Base".
func embeddedType(meta *metadata.Metadata, name, defaultPkg string) *metadata.Type {
	pkg, typeName := splitPkgType(stripPointer(name))
	if typeName == "" {
		return nil
	}
	if pkg == "" {
		pkg = defaultPkg
	}
	return findType(meta, pkg, typeName)
}

// embeddedFieldName is the JSON name an embedded non-struct gets: the type's
// own name, without package or pointer.
func embeddedFieldName(name string) string {
	name = stripPointer(name)
	if i := strings.LastIndexAny(name, "./"); i >= 0 {
		name = name[i+1:]
	}
	return name
}

// isStructType reports whether a declared type is a struct, which decides
// whether embedding it promotes fields or contributes one.
//
// A struct declares fields or embeds something; anything else records its
// underlying type in Target ("string" for `type ID string`). An empty struct
// declares neither, and answers struct — which is right: embedding it promotes
// nothing, and the alternative would publish a property named for it.
func isStructType(meta *metadata.Metadata, typ *metadata.Type) bool {
	if typ == nil {
		return false
	}
	if len(typ.Fields) > 0 || len(typ.Embeds) > 0 {
		return true
	}
	target := strings.TrimSpace(getStringFromPool(meta, typ.Target))
	return target == "" || strings.HasPrefix(target, "struct")
}
