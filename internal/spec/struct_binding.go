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
	"reflect"
	"slices"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// ExtractStructParams documents a whole-struct binding — `c.ShouldBindQuery(&q)`
// — as one parameter per field the binder fills (issue #355).
//
// The idiomatic way to read a query string in gin, echo and fiber is to bind a
// struct and let its tags name the parameters. The string-literal accessor
// patterns cannot see that shape: the call names no parameter at all. Here the
// bound argument is resolved to its declared struct the way a request body is,
// and each StructBinding reads one tag key off its fields.
//
// Nothing is guessed. An argument that does not resolve to a declared struct
// (an `any` passed through a wrapper, a type outside the analysed packages)
// yields no parameters, and so does a field whose type is itself a struct —
// binders flatten those by rules of their own, and a parameter named for the
// outer field would be one no request can satisfy (golden rule #7).
func (p *ParamPatternMatcherImpl) ExtractStructParams(node TrackerNodeInterface, route *RouteInfo) []Parameter {
	if route == nil || route.Metadata == nil {
		return nil
	}
	edge := node.GetEdge()
	if edge == nil || p.pattern.TypeArgIndex >= len(edge.Args) {
		return nil
	}
	typ := p.boundStruct(edge.Args[p.pattern.TypeArgIndex], node, route.Metadata)
	if typ == nil {
		return nil
	}

	var placeholders []string
	var params []Parameter
	for _, binding := range p.pattern.StructBindings {
		if len(binding.Methods) > 0 && !slices.ContainsFunc(binding.Methods, func(m string) bool {
			return strings.EqualFold(m, route.Method)
		}) {
			continue
		}
		if binding.In == "path" && placeholders == nil {
			// The route still carries the framework's own syntax (`:id`).
			openAPIPath, _ := convertPathToOpenAPI(route.Path)
			placeholders = pathPlaceholders(openAPIPath)
		}
		for _, bf := range boundFields(route.Metadata, typ, binding) {
			// A path parameter only exists where the route has a segment for
			// it: a `uri:"id"` field on a route without `:id` binds nothing.
			if binding.In == "path" && !slices.Contains(placeholders, bf.name) {
				continue
			}
			if param := p.structFieldParam(route, bf, binding.In); param != nil {
				params = append(params, *param)
			}
		}
	}
	return params
}

// boundStruct resolves the bound argument to the struct declaration it names,
// following it through a binding wrapper's parameters to the caller's value.
func (p *ParamPatternMatcherImpl) boundStruct(arg *metadata.CallArgument, node TrackerNodeInterface, meta *metadata.Metadata) *metadata.Type {
	typeNode := node
	if resolved, rnode := resolveArgThroughParams(arg, node); resolved != arg && rnode != nil {
		arg, typeNode = resolved, rnode
	}
	goType := p.resolveTypeOrigin(arg, typeNode, p.contextProvider.GetArgumentInfo(arg))
	core := typemodel.Parse(goType).Core()
	if core == nil || core.Name == "" {
		return nil
	}
	typ := typeByName(core.Pkg, core.Name, meta)
	if typ == nil || !isStructType(meta, typ) {
		return nil
	}
	return typ
}

// boundField is one struct field a binding fills, with the package its type
// names resolve against.
type boundField struct {
	name  string
	field metadata.Field
	pkg   string
	depth int
}

// boundFields lists the fields a binding fills, in declaration order, with
// untagged struct embeds promoted the way every supported binder promotes them.
// Where two fields share a wire name the shallowest wins, as Go's selector rule
// has it: an outer field shadows the one an embed would promote.
func boundFields(meta *metadata.Metadata, typ *metadata.Type, binding StructBinding) []boundField {
	var all []boundField
	collectBoundFields(meta, typ, binding, 0, map[*metadata.Type]bool{}, &all)

	shallowest := map[string]int{}
	for _, f := range all {
		if d, ok := shallowest[f.name]; !ok || f.depth < d {
			shallowest[f.name] = f.depth
		}
	}
	out := all[:0]
	kept := map[string]bool{}
	for _, f := range all {
		if f.depth == shallowest[f.name] && !kept[f.name] {
			kept[f.name] = true
			out = append(out, f)
		}
	}
	return out
}

func collectBoundFields(meta *metadata.Metadata, typ *metadata.Type, binding StructBinding, depth int, visited map[*metadata.Type]bool, out *[]boundField) {
	if typ == nil || visited[typ] {
		return
	}
	visited[typ] = true
	pkg := getStringFromPool(meta, typ.Pkg)

	next := 0
	embedsBefore := func(pos int) {
		for ; next < len(typ.Embeds); next++ {
			if embedAt(typ, next) > pos {
				return
			}
			tag := ""
			if next < len(typ.EmbedTags) {
				tag = getStringFromPool(meta, typ.EmbedTags[next])
			}
			if _, tagged := reflect.StructTag(tag).Lookup(binding.Tag); tagged {
				// A tagged embed is a single value under that name, which no
				// binder reads as a scalar parameter.
				continue
			}
			embedded := embeddedType(meta, getStringFromPool(meta, typ.Embeds[next]), pkg)
			if embedded != nil && isStructType(meta, embedded) {
				collectBoundFields(meta, embedded, binding, depth+1, visited, out)
			}
		}
	}
	for i, f := range typ.Fields {
		embedsBefore(i)
		if name := boundFieldName(meta, f, binding); name != "" {
			*out = append(*out, boundField{name: name, field: f, pkg: pkg, depth: depth})
		}
	}
	embedsBefore(len(typ.Fields))
}

// boundFieldName is the wire name a binding reads a field under, or "" when it
// does not read the field.
func boundFieldName(meta *metadata.Metadata, f metadata.Field, binding StructBinding) string {
	goName := getStringFromPool(meta, f.Name)
	if !ast.IsExported(goName) {
		return ""
	}
	value := reflect.StructTag(getStringFromPool(meta, f.Tag)).Get(binding.Tag)
	name, _, _ := strings.Cut(value, ",")
	switch {
	case name == "-":
		return ""
	case name != "":
		return name
	case binding.UntaggedByName:
		// Also the `form:",default=1"` case: a tag that sets options but
		// leaves the name to the field.
		return goName
	}
	return ""
}

// structFieldParam documents one bound field as a parameter.
func (p *ParamPatternMatcherImpl) structFieldParam(route *RouteInfo, bf boundField, in string) *Parameter {
	meta := route.Metadata
	fieldType := getStringFromPool(meta, bf.field.Type)
	if fieldType == "" || bf.field.NestedType != nil {
		return nil
	}
	if !metadata.IsPrimitiveType(fieldType) {
		fieldType = qualifyFieldType(fieldType, bf.pkg)
	}
	// An optional field is spelled as a pointer; the parameter is the value.
	fieldType = stripPointer(fieldType)

	// A struct-typed field is not one parameter. Its declaration is the
	// evidence: a type the metadata does not declare (time.Time, uuid.UUID) is
	// left to the mapper, which knows its scalar form.
	if core := typemodel.Parse(fieldType).Core(); core != nil && core.Name != "" {
		if decl := typeByName(core.Pkg, core.Name, meta); decl != nil && isStructType(meta, decl) &&
			!isInlineExternalType(fieldType, p.cfg, meta) {
			return nil
		}
	}

	schema := mapGoTypeForRoute(route.UsedTypes, fieldType, meta, p.cfg)
	if schema == nil {
		schema = &Schema{Type: "string"}
	}
	param := &Parameter{Name: bf.name, In: in, Schema: schema}

	tag := getStringFromPool(meta, bf.field.Tag)
	sources := []string{tag}
	if key := p.pattern.ValidateTag; key != "" && key != "validate" {
		if rules, ok := reflect.StructTag(tag).Lookup(key); ok {
			sources = append(sources, `validate:"`+rules+`"`)
		}
	}
	for _, src := range sources {
		if c := extractValidationConstraints(src); c != nil {
			// Constraints mutate the schema, and a schema a $ref points at is
			// shared with every other use of the type.
			if schema.Ref == "" {
				applyValidationConstraints(schema, c)
			}
			param.Required = param.Required || c.Required
		}
	}

	if in == "path" {
		param.Required = true
		applyPathConstraint(param, route.Path)
	}
	return param
}
