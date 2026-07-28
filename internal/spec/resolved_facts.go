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
	"strings"

	"github.com/ehabterra/apispec/internal/callgraph"
	"github.com/ehabterra/apispec/internal/metadata"
)

// TypeFactsFor answers the two questions that decide whether a resolved callee
// may be trusted over the recorded one, from the facts metadata already holds.
//
// It lives in the spec layer rather than in metadata or callgraph because it
// joins the two, and neither should learn about the other (golden rule #4:
// metadata records facts, the layer above decides what they mean).
func TypeFactsFor(meta *metadata.Metadata) callgraph.TypeFacts {
	if meta == nil {
		return callgraph.TypeFacts{}
	}
	interfaceKind := meta.StringPool.Get("interface")

	// Both answers are memoized: a large project asks about the same handful of
	// context and interface types at thousands of call sites.
	isInterface := map[string]bool{}
	embeds := map[string]map[string]bool{}

	lookup := func(qualified string) *metadata.Type {
		pkg, name := splitQualified(qualified)
		if pkg == "" || name == "" {
			return nil
		}
		p, ok := meta.Packages[pkg]
		if !ok {
			return nil
		}
		for _, fileName := range meta.SortedFileNames(pkg) {
			file := p.Files[fileName]
			if file == nil {
				continue
			}
			if typ, ok := file.Types[name]; ok {
				return typ
			}
		}
		return nil
	}

	return callgraph.TypeFacts{
		IsInterface: func(qualified string) bool {
			if known, ok := isInterface[qualified]; ok {
				return known
			}
			typ := lookup(qualified)
			answer := typ != nil && typ.Kind == interfaceKind
			isInterface[qualified] = answer
			return answer
		},
		Embeds: func(outer, inner string) bool {
			if known, ok := embeds[outer]; ok {
				return known[inner]
			}
			// Walk the embedding chain once per outer type: a type embedding a type
			// that embeds the target gets its methods too.
			reachable := map[string]bool{}
			pkg, _ := splitQualified(outer)
			queue := []string{outer}
			for len(queue) > 0 {
				current := queue[0]
				queue = queue[1:]
				typ := lookup(current)
				if typ == nil {
					continue
				}
				for _, embedded := range typ.Embeds {
					name := strings.TrimPrefix(meta.StringPool.GetString(embedded), "*")
					if name == "" {
						continue
					}
					qualified := name
					if !strings.Contains(name, ".") && pkg != "" {
						qualified = pkg + "." + name
					}
					if reachable[qualified] {
						continue
					}
					reachable[qualified] = true
					queue = append(queue, qualified)
				}
			}
			embeds[outer] = reachable
			return reachable[inner]
		},
	}
}

// splitQualified splits `pkg/path.Name` into its package and bare type name. The
// split is at the LAST dot, because an import path contains dots of its own
// (`github.com/x/y.Type`).
func splitQualified(qualified string) (pkg, name string) {
	i := strings.LastIndexByte(qualified, '.')
	if i < 0 {
		return "", ""
	}
	return qualified[:i], qualified[i+1:]
}
