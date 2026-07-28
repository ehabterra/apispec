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

	"github.com/ehabterra/apispec/internal/metadata"
)

// entrypointRoots decides which declared entrypoints (issue #220) become extra
// tree roots.
//
// A tree root is normally a main function, because everything a program does is
// reachable from one. A CLI-dispatched program breaks that: the function that
// registers every route is stored in a `cli.Command{Action: …}` field and invoked
// by the library, so no call edge from main reaches it and its whole subtree is
// invisible. Rooting the entrypoint restores it.
//
// Two gates keep this from turning into a pile of spurious roots, and both matter
// on real projects — photoprism declares 55 command literals, of which exactly
// one leads to an HTTP server:
//
//  1. **Already reachable is left alone.** An entrypoint that main can reach is
//     expanded through its real call path, with whatever mount context that path
//     carries. Rooting it again would document the same routes twice.
//  2. **Must be able to reach a route registration.** Answered by the same
//     bottom-up SCC pass the extractor uses for param accessors and middleware,
//     so a subcommand that indexes photos or runs migrations is never expanded.
//     This is what keeps the cost proportional to routing code rather than to
//     the number of subcommands.
//
// Returned keys are sorted: they become tree roots, and root order reaches the
// output (golden rule #1).
func entrypointRoots(meta *metadata.Metadata, candidates []string, routeMatch func(*metadata.CallGraphEdge) bool, logger metadata.VerboseLogger) ([]string, EntrypointStats) {
	if meta == nil || len(candidates) == 0 {
		return nil, EntrypointStats{}
	}

	reachedFromMain := functionsReachableFromMains(meta)
	registers := reachesMatch(meta, routeMatch)

	var out []string
	stats := EntrypointStats{Declared: len(candidates)}
	for _, key := range candidates {
		switch {
		case reachedFromMain[key]:
			stats.AlreadyReachable++
		case !registers[key]:
			stats.NoRoutes++
		default:
			out = append(out, key)
		}
	}
	sort.Strings(out)
	stats.Rooted = len(out)
	if logger != nil {
		// Bounded is fine; silent is not. A project whose routes hang off a
		// dispatcher should be able to see that apispec noticed.
		logger.Printf("Entrypoints: %d declared, %d rooted (%d already reachable, %d register no routes)\n",
			stats.Declared, stats.Rooted, stats.AlreadyReachable, stats.NoRoutes)
	}
	return out, stats
}

// EntrypointStats records what the entrypoint gate decided, so the same numbers
// the --verbose line reports can be shown in the UI: a project whose routes hang
// off a CLI dispatcher stands or falls on this, and "0 rooted, N register no
// routes" is the difference between "apispec never looked" and "apispec looked
// and nothing there registers a route".
type EntrypointStats struct {
	Declared         int
	Rooted           int
	AlreadyReachable int
	NoRoutes         int
}

// functionsReachableFromMains returns every function base key reachable from a
// main function, by forward BFS over the call graph. Linear in edges.
func functionsReachableFromMains(meta *metadata.Metadata) map[string]bool {
	seen := map[string]bool{}
	var queue []string
	for _, edge := range meta.CallGraphRoots() {
		if getString(meta, edge.Caller.Name) != metadata.MainFunc {
			continue
		}
		key := metadata.StripToBase(edge.Caller.BaseID())
		if !seen[key] {
			seen[key] = true
			queue = append(queue, key)
		}
	}
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		for _, edge := range meta.Callers[key] {
			callee := edge.Callee.BaseID()
			if !seen[callee] {
				seen[callee] = true
				queue = append(queue, callee)
			}
		}
	}
	return seen
}

// reachesMatch returns the set of function base keys from which an edge matching
// match is transitively reachable — the tree-side twin of Extractor.reachSet,
// same one-pass-over-the-condensation shape, without needing an extractor.
func reachesMatch(meta *metadata.Metadata, match func(*metadata.CallGraphEdge) bool) map[string]bool {
	scc := metadata.BuildCallGraphSCC(meta)
	compReaches := make([]bool, len(scc.Components))
	for c, comp := range scc.Components {
		reached := false
		for _, member := range comp {
			for _, edge := range meta.Callers[member] {
				if match(edge) {
					reached = true
					break
				}
				if calleeComp, ok := scc.ComponentOf[edge.Callee.BaseID()]; ok && calleeComp != c && compReaches[calleeComp] {
					reached = true
					break
				}
			}
			if reached {
				break
			}
		}
		compReaches[c] = reached
	}
	set := make(map[string]bool)
	for id, c := range scc.ComponentOf {
		if compReaches[c] {
			set[id] = true
		}
	}
	return set
}

// RouteRegistrationMatcher builds the predicate for gate 2 from the effective
// config's route patterns: an edge registers a route when its callee name matches
// a route pattern's call regex.
//
// The receiver IS checked when the pattern scopes one, because a name-only test
// is far too loose to be useful: gin's verb pattern is `^(?i)(GET|POST|…)$`, which
// matches `Get` on a cache, a config, or `http.Get`. On photoprism that admitted
// 24 of 53 entrypoints and cost 25s of expansion; with the receiver honoured only
// genuine router calls qualify.
//
// The direction of error still favours routes: a pattern that scopes no receiver
// falls back to name-only (over-approximating, which merely costs an expansion),
// and a wrapper-router project whose own pattern is configured matches on that
// pattern's receiver instead. What it will not do is invent a root for a
// subcommand that calls `cache.Get`.
func RouteRegistrationMatcher(cfg *APISpecConfig, meta *metadata.Metadata) func(*metadata.CallGraphEdge) bool {
	if cfg == nil || meta == nil {
		return func(*metadata.CallGraphEdge) bool { return false }
	}
	type gate struct{ call, recv, recvExact string }
	gates := make([]gate, 0, len(cfg.Framework.RoutePatterns)+len(cfg.Framework.MountPatterns))
	for _, p := range cfg.Framework.RoutePatterns {
		if p.CallRegex != "" {
			gates = append(gates, gate{p.CallRegex, p.RecvTypeRegex, p.RecvType})
		}
	}
	// Mounts count too: a subcommand whose only routing act is mounting a
	// sub-router still leads to routes.
	for _, p := range cfg.Framework.MountPatterns {
		if p.CallRegex != "" {
			gates = append(gates, gate{p.CallRegex, p.RecvTypeRegex, p.RecvType})
		}
	}
	if len(gates) == 0 {
		return func(*metadata.CallGraphEdge) bool { return false }
	}
	return func(edge *metadata.CallGraphEdge) bool {
		name := getString(meta, edge.Callee.Name)
		if name == "" {
			return false
		}
		// Same fully-qualified form the extractor's matchers build (pkg + "." +
		// recvType): a bare "*RouterGroup" matches no framework's RecvTypeRegex,
		// and comparing the raw string silently rejected every real router call.
		recvType := getString(meta, edge.Callee.RecvType)
		recv := getString(meta, edge.Callee.Pkg)
		switch {
		case recv != "" && recvType != "":
			recv += "." + recvType
		case recvType != "":
			recv = recvType
		}
		for _, g := range gates {
			re, err := cachedRegex(g.call)
			if err != nil || !re.MatchString(name) {
				continue
			}
			switch {
			case g.recv != "":
				if rre, rerr := cachedRegex(g.recv); rerr != nil || !rre.MatchString(recv) {
					continue
				}
			case g.recvExact != "":
				if g.recvExact != recv {
					continue
				}
			}
			return true
		}
		return false
	}
}

// ExpansionStats reports how far tree expansion got.
//
// Truncated is the fact worth surfacing: expansion stopped because it ran out of
// node budget, not because there was nothing left to expand, so the spec is
// incomplete by an unknown amount. On a large project that reads as a working run
// with a suspiciously short route list — gitea documents 5 paths at the default
// limit and 862 with the budget raised — and the only sign was a line on stderr
// (issue #233).
type ExpansionStats struct {
	NodesBuilt int
	Limit      int
	Truncated  bool
}
