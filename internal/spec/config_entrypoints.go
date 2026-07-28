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

import "github.com/ehabterra/apispec/internal/metadata"

// Entrypoint presets for the command libraries that dispatch through a struct
// field (issue #220).
//
// A server-shaped Go program usually starts its HTTP router from a *subcommand*,
// and the call that invokes that subcommand lives inside the CLI library:
//
//	var CmdWeb = &cli.Command{Action: runWeb}   // gitea, photoprism
//	var serveCmd = &cobra.Command{RunE: runServe} // the cobra equivalent
//
// Nothing in the analysed module calls `runWeb`, so without these presets the
// entire route-registration subtree is unreachable and the project documents
// nothing at all.
//
// Presets are keyed on the project's imports and cost nothing when absent, which
// is why this is a preset list rather than an always-on heuristic: a project that
// does not use these libraries pays no analysis for them.
type entrypointBundle struct {
	// Name is for diagnostics only.
	Name string
	// ImportRegexes gate the bundle on the project's imports.
	ImportRegexes []string
	Patterns      []EntrypointPattern
}

func entrypointBundles() []entrypointBundle {
	return []entrypointBundle{
		{
			// urfave/cli — v1 (also its pre-rename import path), v2 and v3. The
			// field is `Action` on both Command and App (v1/v2 have App; v3
			// folded it into Command). `Before`/`After` are lifecycle hooks and
			// are included because a program is free to start its server there,
			// and an entrypoint that registers no route costs nothing.
			Name: "urfave/cli",
			ImportRegexes: []string{
				`^github\.com/urfave/cli(/v\d+)?$`,
				`^github\.com/codegangsta/cli$`,
			},
			Patterns: []EntrypointPattern{
				{
					FieldRegex:    `^(Action|Before|After)$`,
					RecvTypeRegex: `^github\.com/(urfave|codegangsta)/cli(/v\d+)?\.(Command|App)$`,
				},
			},
		},
		{
			// spf13/cobra — `Run`/`RunE` are the command bodies; the Pre/Post
			// hooks are where a program occasionally starts a server.
			Name: "spf13/cobra",
			ImportRegexes: []string{
				`^github\.com/spf13/cobra$`,
			},
			Patterns: []EntrypointPattern{
				{
					FieldRegex:    `^(Run|RunE|PreRun|PreRunE|PostRun|PostRunE|PersistentPreRun|PersistentPreRunE)$`,
					RecvTypeRegex: `^github\.com/spf13/cobra\.Command$`,
				},
			},
		},
		{
			// peterbourgon/ff — ffcli's command body is `Exec`.
			Name: "peterbourgon/ff (ffcli)",
			ImportRegexes: []string{
				`^github\.com/peterbourgon/ff(/v\d+)?/ffcli$`,
			},
			Patterns: []EntrypointPattern{
				{
					FieldRegex:    `^Exec$`,
					RecvTypeRegex: `^github\.com/peterbourgon/ff(/v\d+)?/ffcli\.Command$`,
				},
			},
		},
		{
			// mitchellh/cli dispatches through a factory map
			// (map[string]CommandFactory), so the entrypoint is the factory's
			// value rather than a named field. Recorded here as a deliberate
			// gap: nothing to match on, and guessing would break the
			// honest-over-wrong rule.
			//
			// Same for alecthomas/kong and google/subcommands, which dispatch
			// through an interface METHOD — those already resolve via the
			// existing interface-implementer fan-out and need no entrypoint.
			Name:          "",
			ImportRegexes: nil,
			Patterns:      nil,
		},
	}
}

// ApplyEntrypointPresets appends the entrypoint patterns for whichever command
// libraries the project imports. User-declared patterns keep precedence by
// staying first in the list; the merge is otherwise additive and deduped, so
// applying it twice is a no-op.
//
// Mirrors ApplySecurityPresets: config data only, no engine behaviour, and inert
// for a project that imports none of these libraries.
func ApplyEntrypointPresets(cfg *APISpecConfig, meta *metadata.Metadata) {
	if cfg == nil {
		return
	}
	imports := collectImports(meta)
	if len(imports) == 0 {
		return
	}

	seen := make(map[string]bool, len(cfg.Framework.EntrypointPatterns))
	for _, p := range cfg.Framework.EntrypointPatterns {
		seen[entrypointPatternKey(p)] = true
	}
	for _, bundle := range entrypointBundles() {
		if len(bundle.Patterns) == 0 || !anyImportMatches(imports, bundle.ImportRegexes) {
			continue
		}
		for _, p := range bundle.Patterns {
			if key := entrypointPatternKey(p); !seen[key] {
				seen[key] = true
				cfg.Framework.EntrypointPatterns = append(cfg.Framework.EntrypointPatterns, p)
			}
		}
	}
}

func entrypointPatternKey(p EntrypointPattern) string {
	return p.FieldRegex + "\x00" + p.RecvType + "\x00" + p.RecvTypeRegex
}

// matchesEntrypoint reports whether a (owner type, field) pair is declared as an
// entrypoint. ownerType is the type as metadata renders it, e.g.
// "github.com/urfave/cli/v2.Command".
func matchesEntrypoint(patterns []EntrypointPattern, ownerType, field string) bool {
	if field == "" || ownerType == "" {
		return false
	}
	for _, p := range patterns {
		if p.FieldRegex != "" {
			re, err := cachedRegex(p.FieldRegex)
			if err != nil || !re.MatchString(field) {
				continue
			}
		}
		switch {
		case p.RecvTypeRegex != "":
			re, err := cachedRegex(p.RecvTypeRegex)
			if err != nil || !re.MatchString(ownerType) {
				continue
			}
		case p.RecvType != "":
			if p.RecvType != ownerType {
				continue
			}
		default:
			// Unconstrained owner: would claim every same-named field in the
			// project. Treated as a misconfiguration rather than a wildcard.
			continue
		}
		return true
	}
	return false
}
