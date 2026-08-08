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

	"github.com/ehabterra/apispec/internal/metadata"
)

// mapLiteralSchema builds an object schema from a map composite literal whose
// keys are constant strings — `map[string]any{"cost_codes": items}`, the usual
// Go way to put a named envelope around a payload (issue #295).
//
// Mapping such a literal from its TYPE alone can only ever produce
// `additionalProperties: {type: object}`, because `map[string]any` is all the
// type says. Both facts the caller actually wants are at the literal: the key is
// a constant, and the value carries its own resolved type. Reading them here
// turns an opaque map into the object it is, and — because the value types go
// through mapGoTypeToOpenAPISchema like any other — the payload's own component
// is registered and $ref'd rather than flattened to `{type: object}`.
//
// Returns nil when the argument is not a string-keyed map literal, leaving the
// type-driven mapping in charge. A literal is only partly readable when some
// keys are non-constant (a variable key, a spread) or a value's type did not
// resolve: those become additionalProperties alongside the properties that DID
// resolve, because naming the keys that are known is strictly more information
// than dropping all of them, and claiming the unknown ones do not exist would be
// worse than saying nothing about them (golden rule #7).
// resolveValueType returns the Go type of one map-literal value. It is supplied
// by the caller rather than read off the argument, because a raw
// CallArgument.GetType() is not the answer: a same-package struct ident records
// its type bare ("Meta"), which would key a component under a different naming
// convention than every other type in the same document, and a literal records
// no type at all. The caller owns the sequence that turns an argument into a
// body type, and a value in an envelope deserves the same one as a value written
// on its own.
type resolveValueType func(*metadata.CallArgument) string

func mapLiteralSchema(arg *metadata.CallArgument, resolve resolveValueType, usedTypes map[string]*Schema, meta *metadata.Metadata, cfg *APISpecConfig) *Schema {
	if arg == nil || arg.GetKind() != metadata.KindCompositeLit || arg.X == nil {
		return nil
	}
	mapType := arg.X
	if mapType.GetKind() != metadata.KindMapType || mapType.X == nil {
		return nil
	}
	// OpenAPI object keys are strings; a map keyed by anything else is not an
	// object and the existing fallback already says so.
	if keyType := mapType.X.GetName(); keyType != "string" {
		return nil
	}
	// An empty literal carries no more information than its type.
	if len(arg.Args) == 0 {
		return nil
	}

	props := map[string]*Schema{}
	opaque := false
	for _, elt := range arg.Args {
		key, value, ok := constantKeyValue(elt)
		if !ok {
			opaque = true
			continue
		}
		valueType := resolve(value)
		if valueType == "" {
			// The key is known but the payload's type is not; saying the key
			// exists with an unconstrained schema beats dropping it.
			props[key] = &Schema{}
			continue
		}
		s, _ := mapGoTypeToOpenAPISchema(usedTypes, valueType, meta, cfg, nil)
		if s == nil {
			props[key] = &Schema{}
			continue
		}
		props[key] = s
	}
	if len(props) == 0 {
		return nil
	}

	schema := &Schema{Type: "object", Properties: props}
	if opaque {
		// Some entry could not be read. The keys that were read are documented;
		// the rest stay possible rather than being denied.
		schema.AdditionalProperties = &Schema{}
	}
	return schema
}

// constantKeyValue reports the property name and value of one map-literal entry,
// and whether it could be read at all. Only a `"key": value` pair with a quoted
// string literal key names a property; anything else (a constant ident, a
// computed key) is left to additionalProperties rather than guessed at.
func constantKeyValue(elt *metadata.CallArgument) (string, *metadata.CallArgument, bool) {
	if elt == nil || elt.GetKind() != metadata.KindKeyValue || elt.X == nil || elt.Fun == nil {
		return "", nil, false
	}
	if elt.X.GetKind() != metadata.KindLiteral {
		return "", nil, false
	}
	raw := elt.X.GetValue()
	// Keys reach here as Go source text, so a string key is still quoted.
	// Unquote rather than trim, so an escaped key ("a\"b") is read correctly and
	// a non-string literal key (a number) is rejected instead of being mangled.
	if !strings.HasPrefix(raw, `"`) {
		return "", nil, false
	}
	key, err := strconv.Unquote(raw)
	if err != nil || key == "" {
		return "", nil, false
	}
	return key, elt.Fun, true
}
