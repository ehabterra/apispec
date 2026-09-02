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
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/ehabterra/apispec/internal/metadata"
)

// apiKeyLookupSources maps the sources an auth middleware's lookup grammar names
// to the OpenAPI parameter locations an apiKey scheme can be documented in.
//
// `form` is deliberately absent. Echo and fiber both accept it, and OpenAPI has
// no apiKey location for a form field — so it is treated as unresolvable, which
// keeps the library default and raises the warning rather than documenting a
// location the credential is not sent in (golden rule #7).
var apiKeyLookupSources = map[string]string{
	"header": "header",
	"query":  "query",
	"cookie": "cookie",
}

// parseAPIKeyLookup reads a middleware's configured lookup — the grammar echo
// and fiber share, `"<source>:<name>"` — into the location and name an apiKey
// scheme is documented with. ok is false when the value is empty, malformed, or
// names a source OpenAPI cannot express.
func parseAPIKeyLookup(lookup string) (in, name string, ok bool) {
	source, key, found := strings.Cut(strings.TrimSpace(lookup), ":")
	if !found {
		return "", "", false
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return "", "", false
	}
	in, ok = apiKeyLookupSources[strings.ToLower(strings.TrimSpace(source))]
	if !ok {
		return "", "", false
	}
	return in, key, true
}

// lookupValueFromArgs reads the configured lookup out of the middleware's own
// arguments: field `field` of the composite literal passed as argument `index`.
//
// The two ways of having no value are told apart, because they deserve
// different answers. `set` is false when the middleware was called without that
// configuration at all — echo's `KeyAuth(validator)` takes the validator
// directly, and the field being absent from the literal means the same: the
// library default applies and is correct, so nothing is reported. `set` is true
// with an empty value when the field IS configured and cannot be read — built
// at runtime, read from the environment — where the default is a fallback that
// may well be wrong, and saying so is the point (issue #370).
func lookupValueFromArgs(args []*metadata.CallArgument, index int, field string) (value string, set bool) {
	if field == "" || index < 0 || index >= len(args) {
		return "", false
	}
	lit := unwrapComposite(args[index])
	if lit == nil || lit.GetKind() != metadata.KindCompositeLit {
		return "", false
	}
	for _, elt := range lit.Args {
		if elt == nil || elt.GetKind() != metadata.KindKeyValue || elt.X == nil || elt.Fun == nil {
			continue
		}
		if elt.X.GetName() != field {
			continue
		}
		if elt.Fun.GetKind() != metadata.KindLiteral {
			return "", true // configured, not knowable
		}
		return strings.Trim(elt.Fun.GetValue(), `"`), true
	}
	return "", false
}

// sortedShapes orders shapes so scheme keys are assigned the same way on every
// run — the assignment matters because a collision resolves by suffix.
func sortedShapes(shapes map[apiKeyShape]bool) []apiKeyShape {
	out := make([]apiKeyShape, 0, len(shapes))
	for shape := range shapes {
		out = append(out, shape)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].In != out[j].In {
			return out[i].In < out[j].In
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// reportOverriddenShapes says so when a scheme the user defined themselves is
// contradicted by what the middleware configures.
//
// The user's definition wins — it is a statement of intent, and inference must
// not quietly rewrite it — but silence would hide a real disagreement, so each
// contradicted scheme is named once.
func reportOverriddenShapes(routes []*RouteInfo, explicit map[string]SecurityScheme) {
	if len(explicit) == 0 {
		return
	}
	reported := map[string]bool{}
	var names []string
	observed := map[string]apiKeyShape{}
	for _, route := range routes {
		if route == nil {
			continue
		}
		for name, shape := range route.SecuritySchemeShapes {
			def, written := explicit[name]
			if !written || reported[name] {
				continue
			}
			if def.In == shape.In && def.Name == shape.Name {
				continue // agrees; nothing to say
			}
			reported[name] = true
			names = append(names, name)
			observed[name] = shape
		}
	}
	sort.Strings(names) // one line per scheme, in a stable order
	for _, name := range names {
		def, shape := explicit[name], observed[name]
		log.Printf("[security] securitySchemes.%s is defined as {in: %s, name: %s}, but the middleware "+
			"reads {in: %s, name: %s} — keeping your definition",
			name, def.In, def.Name, shape.In, shape.Name)
	}
}

// apiKeySchemeKey names the scheme for one resolved lookup shape.
//
// The base name is kept while a project uses ONE shape, which is the ordinary
// case and keeps `apiKeyAuth` in the document. Where several shapes exist the
// base name is suffixed for every one of them rather than handed to whichever
// was seen first: two groups reading different places are two schemes, and
// letting one own the plain name would make it look like the project's single
// answer (issue #370).
func apiKeySchemeKey(base, in, name string) string {
	return base + camelSegment(in) + camelSegment(name)
}

// camelSegment renders a lookup part as one CamelCase word: `api_key` ->
// `ApiKey`, `X-API-Key` -> `XApiKey`. Non-alphanumerics separate words and are
// dropped, so the result is safe as a component key.
func camelSegment(s string) string {
	var b strings.Builder
	upperNext := true
	for _, r := range s {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upperNext {
				b.WriteRune(unicode.ToUpper(r))
				upperNext = false
			} else {
				b.WriteRune(unicode.ToLower(r))
			}
		default:
			upperNext = true
		}
	}
	return b.String()
}

// specializeAPIKeySchemes rewrites the routes' security requirements to name a
// scheme that says where the credential actually travels, and returns the
// definitions for the names it used (issue #370).
//
// Two shapes are possible for one declared scheme name, and both are handled
// here rather than at the point of resolution, because the choice depends on
// what the WHOLE project does:
//
//   - one shape across the project — the ordinary case — keeps the declared
//     name (`apiKeyAuth`) and simply corrects its `in`/`name`;
//   - several shapes split into one scheme each, named after the location and
//     key, with none of them keeping the plain name. Handing it to whichever
//     route was seen first would read as the project's single answer, and would
//     depend on walk order (golden rule #1).
//
// A route that references the scheme with NO shape counts as one of those
// shapes: it is the library default, which is a different place from a
// configured one. Without that, a project with one configured group and one
// default group redefined the shared name as the configured shape and
// documented the default group's credential in the wrong place — measured on
// exactly that shape while building this.
func specializeAPIKeySchemes(routes []*RouteInfo, base, explicit map[string]SecurityScheme) map[string]SecurityScheme {
	// Collect the distinct shapes per declared scheme name, and whether the name
	// is also referenced by a route that resolved no shape at all.
	byScheme := map[string]map[apiKeyShape]bool{}
	atDefault := map[string]bool{}
	for _, route := range routes {
		if route == nil {
			continue
		}
		for name, shape := range route.SecuritySchemeShapes {
			if _, written := explicit[name]; written {
				continue // the user's own definition governs this scheme
			}
			if byScheme[name] == nil {
				byScheme[name] = map[apiKeyShape]bool{}
			}
			byScheme[name][shape] = true
		}
		for _, req := range route.Security {
			for name := range req {
				if _, shaped := route.SecuritySchemeShapes[name]; !shaped {
					atDefault[name] = true
				}
			}
		}
	}
	reportOverriddenShapes(routes, explicit)
	if len(byScheme) == 0 {
		return nil
	}

	// The name each (scheme, shape) pair is emitted under.
	renames := map[string]map[apiKeyShape]string{}
	defs := map[string]SecurityScheme{}
	for name, shapes := range byScheme {
		renames[name] = map[apiKeyShape]string{}
		for _, shape := range sortedShapes(shapes) {
			key := name
			if len(shapes) > 1 || atDefault[name] {
				// Two lookups differing only in punctuation (`api_key` and
				// `api-key`) render to one key, and one definition would then
				// overwrite the other while both routes pointed at it. Shapes are
				// keyed in sorted order and a collision takes the next free
				// suffix, so the assignment is stable across runs (golden rule #1).
				key = apiKeySchemeKey(name, shape.In, shape.Name)
				for n := 2; ; n++ {
					if _, taken := defs[key]; !taken {
						break
					}
					key = apiKeySchemeKey(name, shape.In, shape.Name) + strconv.Itoa(n)
				}
			}
			renames[name][shape] = key
			def := base[name]
			if def.Type == "" {
				def.Type = "apiKey"
			}
			def.In, def.Name = shape.In, shape.Name
			defs[key] = def
		}
	}

	for _, route := range routes {
		if route == nil || len(route.SecuritySchemeShapes) == 0 {
			continue
		}
		for i, req := range route.Security {
			for name, scopes := range req {
				shape, ok := route.SecuritySchemeShapes[name]
				if !ok {
					continue
				}
				key := renames[name][shape]
				if key == "" || key == name {
					continue
				}
				delete(route.Security[i], name)
				route.Security[i][key] = scopes
			}
		}
	}
	return defs
}
