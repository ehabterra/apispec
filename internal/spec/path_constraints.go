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
	"strconv"
	"strings"
)

// constraintRule is what one named route constraint states about a parameter.
//
// A rule is DATA, not logic: `argFields` names the schema fields the
// constraint's own arguments fill, in order, so `range(1,10)` and `minLen(3)`
// go through the same code as `int`. Adding a router's vocabulary is a table
// entry (golden rule #5).
type constraintRule struct {
	typ       string
	format    string
	pattern   string
	argFields []string
}

// pathConstraintRules maps a named route constraint to what it states.
//
// Not gated on a framework, for the same reason stripAngleConstraints is not:
// the syntax is unambiguous, so a router that spells `uuid` the way another
// spells `guid` costs a row rather than a branch. Only fiber writes this form
// today; both spellings are listed because the vocabulary, not the router, is
// the thing being described.
//
// Absent on purpose: `datetime(layout)`. Its argument is a Go time LAYOUT, and
// which OpenAPI format it implies depends on what the layout contains — a
// date-only layout is `date`, one with a clock is `date-time`, and a layout can
// be neither. That is a parse, not a table row, and guessing one of the two
// would be wrong for the other (golden rule #7), so the constraint strips and
// leaves `type: string`.
var pathConstraintRules = map[string]constraintRule{
	"int":   {typ: "integer"},
	"bool":  {typ: "boolean"},
	"float": {typ: "number"},
	"alpha": {typ: "string", pattern: "^[A-Za-z]+$"},
	"guid":  {typ: "string", format: "uuid"},
	"uuid":  {typ: "string", format: "uuid"},

	"minLen": {typ: "string", argFields: []string{"minLength"}},
	"maxLen": {typ: "string", argFields: []string{"maxLength"}},
	// An exact length is both bounds; OpenAPI has no single "length".
	"len":   {typ: "string", argFields: []string{"minLength", "maxLength"}},
	"min":   {typ: "integer", argFields: []string{"minimum"}},
	"max":   {typ: "integer", argFields: []string{"maximum"}},
	"range": {typ: "integer", argFields: []string{"minimum", "maximum"}},
	"regex": {typ: "string", argFields: []string{"pattern"}},
}

// pathParamConstraints returns, for each parameter the RAW path constrains, the
// schema that constraint states and whether it was understood.
//
// The two forms a supported router writes:
//
//	/items/{id:[0-9]+}   gorilla/mux, chi — a regex, surfaced as `pattern`
//	/items/:id<int>      fiber — a named constraint, possibly `;`-chained
//
// A parameter with an UNRECOGNISED constraint gets an entry with understood
// false: the tail is still stripped from the path, but nothing is claimed about
// the value (golden rule #7).
func pathParamConstraints(rawPath string) map[string]pathParamConstraint {
	out := map[string]pathParamConstraint{}

	// The mux/chi regex form. Unchanged behaviour: a pattern, no type — and
	// `described` stays false, because a regex says what the value looks like
	// without saying what it IS. Reading a type out of a numeric regex is #345.
	for name, pattern := range pathParamPatterns(rawPath) {
		out[name] = pathParamConstraint{schema: &Schema{Type: "string", Pattern: pattern}}
	}

	for name, spec := range angleConstraints(rawPath) {
		c := out[name]
		if c.schema == nil {
			c.schema = &Schema{Type: "string"}
		}
		c.described = applyConstraintSpec(c.schema, spec)
		out[name] = c
	}

	if len(out) == 0 {
		return nil
	}
	return out
}

// pathParamConstraint is what a route pattern states about one parameter.
//
// described separates "the router constrained this" from "we understood the
// constraint". Only the second is evidence about the VALUE, and only the second
// answers the "not found in the code" warning: a parameter the route pattern
// types is better attested than one a handler happens to read, while an
// unrecognised constraint has told us nothing (issue #357).
type pathParamConstraint struct {
	schema    *Schema
	described bool
}

// clone returns a copy of the constraint's schema, because a parameter schema
// is written into every operation that has that placeholder and callers must
// not share one pointer between them.
func (c pathParamConstraint) clone() *Schema {
	if c.schema == nil {
		return &Schema{Type: "string"}
	}
	s := *c.schema
	return &s
}

// angleConstraints maps a parameter name to the text of its `<...>` tail.
func angleConstraints(rawPath string) map[string]string {
	if !strings.ContainsRune(rawPath, '<') {
		return nil
	}
	out := map[string]string{}
	for i := 0; i < len(rawPath); i++ {
		if rawPath[i] != '<' {
			continue
		}
		// The name is the identifier immediately before the '<', however the
		// router spells the placeholder (`:id<int>` or `{id<int>}`).
		j := i
		for j > 0 && isParamNameByte(rawPath[j-1]) {
			j--
		}
		name := rawPath[j:i]
		end := angleTailEnd(rawPath, i)
		if end < 0 {
			break // unterminated: nothing reliable to read
		}
		if name != "" {
			out[name] = rawPath[i+1 : end]
		}
		i = end
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func isParamNameByte(b byte) bool {
	return b == '_' || (b >= '0' && b <= '9') || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z')
}

// angleTailEnd returns the index of the '>' closing the tail opened at start,
// or -1. Parentheses are tracked because a constraint's argument is arbitrary
// text — `regex([^>]+)` closes a bracket inside its own argument — which is the
// same rule stripAngleConstraints applies when removing the tail.
func angleTailEnd(s string, start int) int {
	depth := 0
	for i := start + 1; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '>':
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// applyConstraintSpec applies a `;`-separated constraint list to schema and
// reports whether any of it was understood.
func applyConstraintSpec(schema *Schema, spec string) bool {
	understood := false
	for _, part := range splitConstraintList(spec) {
		name, args := splitConstraintCall(part)
		rule, ok := pathConstraintRules[name]
		if !ok {
			continue
		}
		if rule.typ != "" {
			schema.Type = rule.typ
		}
		if rule.format != "" {
			schema.Format = rule.format
		}
		if rule.pattern != "" && schema.Pattern == "" {
			schema.Pattern = rule.pattern
		}
		applyConstraintArgs(schema, rule, args)
		understood = true
	}
	return understood
}

// applyConstraintArgs fills the schema fields a rule's arguments target. A
// constraint written with the wrong arity states nothing extra rather than
// half-filling the schema.
func applyConstraintArgs(schema *Schema, rule constraintRule, args []string) {
	if len(rule.argFields) == 0 || len(args) == 0 {
		return
	}
	for i, field := range rule.argFields {
		// len(n) fills two fields from one argument.
		arg := args[len(args)-1]
		if i < len(args) {
			arg = args[i]
		}
		switch field {
		case "pattern":
			schema.Pattern = arg
		case "minLength", "maxLength", "minimum", "maximum":
			n, err := strconv.ParseFloat(arg, 64)
			if err != nil {
				continue
			}
			switch field {
			case "minLength":
				schema.MinLength = int(n)
			case "maxLength":
				schema.MaxLength = int(n)
			case "minimum":
				schema.Minimum = n
			case "maximum":
				schema.Maximum = n
			}
		}
	}
}

// splitConstraintList splits on ';' at paren depth zero, so a ';' inside
// `regex(a;b)` does not split the list.
func splitConstraintList(spec string) []string {
	var out []string
	depth, start := 0, 0
	for i := 0; i < len(spec); i++ {
		switch spec[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ';':
			if depth == 0 {
				out = append(out, strings.TrimSpace(spec[start:i]))
				start = i + 1
			}
		}
	}
	if s := strings.TrimSpace(spec[start:]); s != "" {
		out = append(out, s)
	}
	return out
}

// splitConstraintCall splits `name(a,b)` into its name and arguments. Arguments
// are split on ',' at depth zero so a nested call keeps its own.
func splitConstraintCall(part string) (string, []string) {
	open := strings.IndexByte(part, '(')
	if open < 0 || !strings.HasSuffix(part, ")") {
		return part, nil
	}
	name := part[:open]
	inner := part[open+1 : len(part)-1]
	if inner == "" {
		return name, nil
	}
	var args []string
	depth, start := 0, 0
	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				args = append(args, strings.TrimSpace(inner[start:i]))
				start = i + 1
			}
		}
	}
	args = append(args, strings.TrimSpace(inner[start:]))
	return name, args
}
