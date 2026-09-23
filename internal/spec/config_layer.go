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
	"fmt"
	"log"
	"reflect"
	"sort"
	"strings"
)

// layerFrameworkLists turns the lists a config file names under `framework:`
// from replacements of the built-in lists into additions to them (issue #571).
//
// Decoding a file onto the composed defaults merges mappings key by key, but a
// YAML sequence decodes into a fresh slice, so every list the file named
// REPLACED the built-in one. A config adding one response pattern for a house
// helper therefore lost every built-in response pattern, and a config frozen by
// --output-config kept replacing each later release's lists with its own, so
// nothing a release added ever took effect for that user. Both were silent.
//
// before is the framework block as it stood before the file was decoded onto
// it, own is the framework block the file states on its own, and cfg is the
// result of decoding the file onto the defaults, which this repairs in place.
// For each list the file names with at least one entry:
//
//   - its entries come FIRST. Response, request and param matchers take the
//     first pattern that matches, so an entry the user wrote wins wherever it
//     and a built-in both apply, and the built-in still answers for the calls
//     the user's entry does not claim. Route, mount and security matchers
//     choose the most specific pattern instead, so their added entries are
//     also flagged (markFromConfig) to outrank every built-in.
//   - an entry identical to a built-in is dropped rather than moved, so the
//     built-ins keep their relative order. A current --output-config export
//     therefore reproduces the defaults exactly instead of reshuffling them.
//
// A list written empty (`routePatterns: []`) still means none, as before, and
// framework.replaceDefaults names the lists that should replace outright.
func layerFrameworkLists(cfg *APISpecConfig, before, own FrameworkConfig) error {
	replace := map[string]bool{}
	for _, name := range own.ReplaceDefaults {
		replace[name] = true
	}
	known := map[string]bool{}
	added := map[string]int{}
	layerLists(reflect.ValueOf(&cfg.Framework).Elem(), reflect.ValueOf(before), reflect.ValueOf(own), "", replace, known, added)
	markFromConfig(&cfg.Framework, added)

	var unknown []string
	for name := range replace {
		if !known[name] {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		names := make([]string, 0, len(known))
		for name := range known {
			names = append(names, name)
		}
		sort.Strings(names)
		// An error rather than a warning: a misspelt name here would leave the
		// list merged while its author believes it replaced, which is the
		// silent divergence this whole function exists to end.
		return fmt.Errorf("framework.replaceDefaults: unknown list(s) %s; valid names are %s",
			strings.Join(unknown, ", "), strings.Join(names, ", "))
	}
	return nil
}

// layerLists walks one struct level of FrameworkConfig. It recurses into
// nested structs (requestContext, responseContext, credentialReads) and stops
// at a list's elements: a pattern's own lists (calleePkgPatterns, …) belong to
// that pattern and are never merged with another pattern's.
func layerLists(out, before, own reflect.Value, prefix string, replace, known map[string]bool, added map[string]int) {
	typ := out.Type()
	for i := 0; i < typ.NumField(); i++ {
		field := typ.Field(i)
		name := yamlName(field)
		if name == "" || name == "replaceDefaults" {
			continue
		}
		path := prefix + name
		switch field.Type.Kind() {
		case reflect.Struct:
			layerLists(out.Field(i), before.Field(i), own.Field(i), path+".", replace, known, added)
		case reflect.Slice:
			known[path] = true
			if own.Field(i).Len() == 0 || replace[path] {
				continue
			}
			merged := layerList(own.Field(i), before.Field(i), path)
			added[path] = merged.Len() - before.Field(i).Len()
			out.Field(i).Set(merged)
		}
	}
}

// markFromConfig flags the entries a file added to the lists whose matchers
// choose by specificity rather than by position — route, mount and security —
// so the user's entry wins an overlap there too (see RoutePattern.fromConfig).
// The added entries are the first added[path] of each layered list.
func markFromConfig(f *FrameworkConfig, added map[string]int) {
	for i := 0; i < added["routePatterns"]; i++ {
		f.RoutePatterns[i].fromConfig = true
	}
	for i := 0; i < added["mountPatterns"]; i++ {
		f.MountPatterns[i].fromConfig = true
	}
	for i := 0; i < added["securityPatterns"]; i++ {
		f.SecurityPatterns[i].fromConfig = true
	}
}

// layerList puts the user's entries ahead of the built-ins, dropping the
// user's copies of built-ins (see layerFrameworkLists for the order).
func layerList(own, before reflect.Value, path string) reflect.Value {
	merged := reflect.MakeSlice(own.Type(), 0, own.Len()+before.Len())
	for i := 0; i < own.Len(); i++ {
		entry := own.Index(i)
		if containsEqual(before, entry) {
			continue
		}
		merged = reflect.Append(merged, entry)
		reportShadowedBuiltin(path, i, entry, before)
	}
	return reflect.AppendSlice(merged, before)
}

// reportShadowedBuiltin says when a user's entry takes precedence over a
// built-in that matches the same call name but is configured differently.
//
// That is sometimes the point — a house override of a built-in — and so it is
// reported rather than refused. But it is also exactly what a file exported by
// an EARLIER release looks like: the old variant of a pattern this release has
// since improved (a destination gate, a receiver scope) keeps winning over the
// new one, and nothing else would ever say so.
func reportShadowedBuiltin(path string, index int, entry, before reflect.Value) {
	call := callRegexOf(entry)
	if call == "" {
		return
	}
	for i := 0; i < before.Len(); i++ {
		if callRegexOf(before.Index(i)) == call {
			log.Printf("[config] framework.%s[%d] (callRegex %q) takes precedence over a built-in pattern for the same call; "+
				"if this file was exported with --output-config by an earlier release, delete the entry to use this release's",
				path, index, call)
			return
		}
	}
}

// callRegexOf returns a pattern's callRegex, or "" for list elements that
// have none (plain strings, sentinels, body readers).
func callRegexOf(v reflect.Value) string {
	if v.Kind() != reflect.Struct {
		return ""
	}
	f := v.FieldByName("CallRegex")
	if !f.IsValid() || f.Kind() != reflect.String {
		return ""
	}
	return f.String()
}

func containsEqual(list, entry reflect.Value) bool {
	for i := 0; i < list.Len(); i++ {
		if reflect.DeepEqual(list.Index(i).Interface(), entry.Interface()) {
			return true
		}
	}
	return false
}

// yamlName is the key a field is written under, or "" for one that is never
// read from a file.
func yamlName(field reflect.StructField) string {
	if !field.IsExported() {
		return ""
	}
	tag := strings.Split(field.Tag.Get("yaml"), ",")[0]
	if tag == "-" {
		return ""
	}
	if tag == "" {
		return strings.ToLower(field.Name)
	}
	return tag
}

// layerExternalTypes does for the top-level externalTypes list what
// layerFrameworkLists does for the pattern lists: the framework presets ship
// entries (gin.H, fiber.Map), and a config adding one type of its own used to
// drop them. Entries are keyed by type name — the one field lookups match on —
// so the user's entry replaces a built-in of the same name, and the rest are
// kept in order after the user's.
func layerExternalTypes(cfg *APISpecConfig, before, own []ExternalType) {
	if len(own) == 0 {
		return
	}
	named := make(map[string]bool, len(own))
	merged := make([]ExternalType, 0, len(own)+len(before))
	for _, e := range own {
		named[e.Name] = true
		merged = append(merged, e)
	}
	for _, e := range before {
		if !named[e.Name] {
			merged = append(merged, e)
		}
	}
	cfg.ExternalTypes = merged
}
