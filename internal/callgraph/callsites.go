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

package callgraph

import (
	"go/token"
	"slices"
	"sort"
	"strings"
)

// CallSite is one resolved call, identified by WHERE it is written rather than
// by who writes it.
//
// The position is the join key to metadata's call graph, and it is the join key
// on purpose. Joining by function identity does not work for the code that
// matters most: metadata names a closure by where it appears (`FuncLit:<pos>`)
// while SSA names it after its parent (`pkg.parent$1`), and measured over two
// real projects those two namings overlap in exactly ZERO cases — 0 of 1,625
// closures on a 3,000-file module. Since routes, handlers and middleware are
// overwhelmingly written as closures, a function-keyed join is blind to the
// interesting half of the program. A call site has one position in one FileSet,
// so joining there sidesteps naming entirely.
type CallSite struct {
	// CallerID is the FunctionID of the enclosing function.
	CallerID string
	// CalleeID is the FunctionID of the resolved target. For an interface or
	// promoted method this is the CONCRETE target VTA resolved, which is the
	// whole reason to consult this graph.
	CalleeID string
	// Position is where the call is written, in the FileSet the packages were
	// loaded with — the same one metadata records positions from.
	Position token.Position
}

// CallSites returns every resolved call that has a source position, sorted so
// the result is identical across runs (golden rule #1: anything that can reach
// the output is ordered).
//
// Synthetic edges — a wrapper the compiler generates, a call from an intrinsic —
// have no Site and are skipped: they exist in no source file, so nothing can join
// to them.
func (r *Resolved) CallSites() []CallSite {
	if r == nil || r.Graph == nil {
		return nil
	}

	var out []CallSite
	for fn, node := range r.Graph.Nodes {
		callerID := FunctionID(fn)
		if callerID == "" {
			continue
		}
		for _, edge := range node.Out {
			if edge.Site == nil || edge.Callee == nil || edge.Callee.Func == nil {
				continue
			}
			pos := edge.Site.Pos()
			if !pos.IsValid() {
				continue
			}
			calleeID := FunctionID(edge.Callee.Func)
			if calleeID == "" {
				continue
			}
			out = append(out, CallSite{
				CallerID: callerID,
				CalleeID: calleeID,
				Position: r.Prog.Fset.Position(pos),
			})
		}
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].Position.Filename != out[j].Position.Filename {
			return out[i].Position.Filename < out[j].Position.Filename
		}
		if out[i].Position.Offset != out[j].Position.Offset {
			return out[i].Position.Offset < out[j].Position.Offset
		}
		if out[i].CallerID != out[j].CallerID {
			return out[i].CallerID < out[j].CallerID
		}
		return out[i].CalleeID < out[j].CalleeID
	})
	return out
}

// CalleesAt indexes resolved targets by SiteKey, so a metadata edge can look up
// what the call it records really resolves to.
//
// A key maps to SEVERAL targets when VTA cannot narrow an interface call to one
// implementation. Keeping all of them is the honest answer (golden rule #7) and
// the caller decides what to do with an ambiguous site; collapsing to "the first
// one" here would silently pick a concrete type the program may never use.
//
// Because the key is per LINE, two distinct calls to the same bare name written
// on one line — `a.Read(); b.Read()` — share a key and pool their targets, so a
// call that VTA resolved unambiguously can be reported as ambiguous. That errs in
// the safe direction: the caller leaves an ambiguous site exactly as recorded, so
// the cost is a rewrite not made, never a wrong one. It cannot be fixed by
// keeping the column, which is the one thing the two graphs never agree on (see
// SiteKey); distinguishing those calls would need the caller to match on
// something else about the site, such as its receiver.
func (r *Resolved) CalleesAt() map[string][]string {
	sites := r.CallSites()
	index := make(map[string][]string, len(sites))
	for _, site := range sites {
		key := SiteKey(site.Position.String(), site.CalleeID)
		if key == "" {
			continue
		}
		if !slices.Contains(index[key], site.CalleeID) {
			index[key] = append(index[key], site.CalleeID)
		}
	}
	return index
}

// SiteKey builds the join key shared by both graphs: the line a call is written
// on, plus the bare name of what it calls.
//
// The COLUMN is deliberately dropped. The two graphs do not agree on it and never
// will: metadata records a call at the start of the statement that contains it,
// while SSA records each call at its own opening parenthesis. Measured on a
// fixture, `json.NewEncoder(w).Encode(v)` is two metadata edges both at column 6
// and two SSA sites at columns 21 and 36 — so a column-exact join matches nothing
// at all (0 of 49 edges), while the file and line always agree.
//
// The bare callee name is what separates several calls written on one line, and
// it is the right discriminator rather than the full ID precisely BECAUSE the two
// graphs are expected to disagree about the qualified callee: an interface call
// that metadata records as `Storage.List` is exactly the one VTA resolves to
// `s3driver.List`, and the shared `List` is what lets the disagreement be seen
// instead of missed.
func SiteKey(position, calleeID string) string {
	line := lineOf(position)
	if line == "" {
		return ""
	}
	name := BareName(calleeID)
	if name == "" {
		return ""
	}
	return line + "#" + name
}

// BareName returns the last dot-separated segment of a function ID — the method
// or function name without its package or receiver.
func BareName(id string) string {
	if i := strings.LastIndexByte(id, '.'); i >= 0 {
		return id[i+1:]
	}
	return id
}

// lineOf strips the trailing `:column` from a `file:line:col` position, leaving
// `file:line`. It counts from the right so a Windows drive letter (`C:\...`),
// which puts a colon inside the filename, survives.
func lineOf(position string) string {
	last := strings.LastIndexByte(position, ':')
	if last < 0 {
		return ""
	}
	if prev := strings.LastIndexByte(position[:last], ':'); prev >= 0 {
		return position[:last]
	}
	// Only one colon: the position carries no column, so it is already file:line.
	return position
}
