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
	"strings"

	"github.com/ehabterra/apispec/internal/callgraph"
	"github.com/ehabterra/apispec/internal/metadata"
)

// ResolvedCalleeStats reports what the resolved graph changed, so a run can say
// it out loud rather than silently documenting a different program.
type ResolvedCalleeStats struct {
	Joined      int // metadata edges the resolved graph also has a call at
	Interface   int // …rewritten to the single implementation of an interface method
	Promoted    int // …rewritten to the type that declares a promoted method
	Ambiguous   int // …left alone: several implementations
	Unexplained int // …left alone: no relation explains the difference
}

// Line renders the stats for --verbose.
func (s ResolvedCalleeStats) Line() string {
	return fmt.Sprintf("Resolved call graph: %d call sites joined, %d rewritten (%d interface, %d promoted), %d left ambiguous, %d unexplained",
		s.Joined, s.Interface+s.Promoted, s.Interface, s.Promoted, s.Ambiguous, s.Unexplained)
}

// ApplyResolvedCallees points metadata's call edges at the callee the resolved
// graph proves they reach.
//
// This is the step where SSA+VTA starts to matter. Metadata records what the
// source says — `store.List(...)` is a call on whatever `store` is declared as —
// while VTA knows which function actually runs. Two differences are worth acting
// on, and both were counted before they were trusted (docs/TRACKER_REDESIGN.md):
//
//   - an interface method with exactly ONE implementation, so the concrete type's
//     patterns and schemas apply (2,880 sites on gitea);
//   - a method reached through embedding, rewritten to the type that declares it,
//     which is how a pattern scoped to the declaring type matches a call written
//     on the embedding one (4,490 sites on gitea — the shape #236 had to emulate
//     by widening receiver regexes by hand).
//
// Two are deliberately NOT acted on. An interface with several implementations
// stays as the interface, because choosing one would invent a concrete type the
// program may never use (golden rule #7). A difference no relation explains is a
// join that landed on the wrong call, and acting on it would corrupt the graph
// (1,230 sites on gitea — 12%, far too many to wave through).
//
// The edge's arguments, assignments and chain links are untouched: the resolved
// graph is the authority on WHICH function a site calls, and metadata remains the
// authority on the call itself.
func ApplyResolvedCallees(meta *metadata.Metadata, resolved *callgraph.Resolved, logger metadata.VerboseLogger) ResolvedCalleeStats {
	var stats ResolvedCalleeStats
	if meta == nil || resolved == nil {
		return stats
	}

	byPosition := resolved.CalleesAt()
	if len(byPosition) == 0 {
		return stats
	}

	stats = applyResolvedCallees(meta, byPosition)
	if logger != nil {
		logger.Printf("%s\n", stats.Line())
	}
	return stats
}

// applyResolvedCallees is ApplyResolvedCallees over an already-built index, which
// is the seam the classification rules are tested through: every class needs a
// call site shaped a particular way, and building an SSA program per case would
// test x/tools rather than these rules.
func applyResolvedCallees(meta *metadata.Metadata, byPosition map[string][]string) ResolvedCalleeStats {
	var stats ResolvedCalleeStats
	facts := TypeFactsFor(meta)

	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		position := meta.StringPool.GetString(edge.Position)
		if position == "" {
			continue
		}
		recorded := edge.Callee.BaseID()
		targets, ok := byPosition[callgraph.SiteKey(position, recorded)]
		if !ok {
			continue
		}
		stats.Joined++

		if containsTarget(targets, recorded) {
			continue // the graphs agree; nothing to do
		}

		switch callgraph.Classify(recorded, targets, facts) {
		case callgraph.DisagreeInterface:
			if rewriteCallee(meta, &edge.Callee, targets[0]) {
				stats.Interface++
			}
		case callgraph.DisagreePromoted:
			if rewriteCallee(meta, &edge.Callee, targets[0]) {
				stats.Promoted++
			}
		case callgraph.DisagreeAmbiguous:
			stats.Ambiguous++
		default:
			stats.Unexplained++
		}
	}

	return stats
}

// rewriteCallee repoints a call at the resolved target, keeping everything about
// the call site that is not the callee's identity.
//
// A fresh Call is built rather than the fields patched in place because Call
// memoizes its own identifiers: mutating Pkg or RecvType behind a populated
// baseIDCache would leave the edge answering with its old identity forever.
//
// The receiver being replaced is CARRIED, not discarded. A pattern config is
// scoped to interface names, and resolving is precisely the act of replacing
// those names with concrete ones — so a rewrite that dropped the written
// receiver would silently stop every interface-scoped pattern from matching and
// every interface-scoped exclusion from excluding (issue #260). Both facts are
// true of the call and matching accepts either; see recvForms.
func rewriteCallee(meta *metadata.Metadata, callee *metadata.Call, target string) bool {
	pkg, recvType, name := splitTarget(target)
	if pkg == "" || name == "" {
		return false
	}

	replacement := metadata.Call{
		Meta:         meta,
		Edge:         callee.Edge,
		Name:         meta.StringPool.Get(name),
		Pkg:          meta.StringPool.Get(pkg),
		Position:     callee.Position,
		Scope:        callee.Scope,
		SignatureStr: callee.SignatureStr,
	}
	if recvType != "" {
		replacement.RecvType = meta.StringPool.Get(recvType)
	}
	// Only when it actually differs: recording an identical second form would
	// make every receiver test compare the same string twice.
	if written := callee.RecvType; written >= 0 && written != replacement.RecvType {
		replacement.SetWrittenRecvType(written)
	} else {
		replacement.SetWrittenRecvType(callee.WrittenRecvType())
	}
	*callee = replacement
	return true
}

// splitTarget takes a resolved function ID apart into the pieces a Call records.
//
// `pkg/path.Type.Method` is a method and `pkg/path.Func` is a plain function. The
// two are told apart by the case of the segment before the name: a receiver type
// is exported or not, but it is a TYPE, and the import path element before it is
// lower-case by convention. That heuristic is not needed here — the resolved ID
// is built by FunctionID, which emits exactly these two shapes — so the split is
// simply "three dot-separated tails means a method".
func splitTarget(target string) (pkg, recvType, name string) {
	lastDot := strings.LastIndexByte(target, '.')
	if lastDot < 0 {
		return "", "", ""
	}
	name = target[lastDot+1:]
	head := target[:lastDot]

	// A method's head ends in `.Type`, where Type has no slash after the last dot.
	if prevDot := strings.LastIndexByte(head, '.'); prevDot >= 0 {
		candidate := head[prevDot+1:]
		if candidate != "" && !strings.Contains(candidate, "/") {
			return head[:prevDot], candidate, name
		}
	}
	return head, "", name
}

func containsTarget(targets []string, want string) bool {
	for _, target := range targets {
		if target == want {
			return true
		}
	}
	return false
}
