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

// Control-flow queries over the block ranges metadata records (Function.Blocks).
//
// The question this answers is "if execution reached B, must it have passed
// through A?" — which is what decides whether a status write can claim a body
// write. Source order alone says only that A is written above B, and that is a
// different thing:
//
//	if err != nil {
//	    w.WriteHeader(400)          // A
//	    return
//	}
//	json.NewEncoder(w).Encode(user) // B — reached only when A did NOT run
//
// A is written first, so an order-based pairing gives B a 400 it can never
// carry. A cannot dominate B because A sits inside a region B is outside of,
// which is exactly what block ranges make visible.
//
// The rule is deliberately one-directional: it rejects a pairing, never invents
// one. Where the facts are missing (no block recorded, a position that will not
// parse) it answers "dominates", leaving the existing behaviour in place —
// missing facts must not silently change a spec (golden rule #7).

// codePos is a source position: the file plus a (line, column) point in it.
type codePos struct {
	file string
	line int
	col  int
}

func (p codePos) valid() bool { return p.file != "" && p.line > 0 }

// beforeOrAt reports whether p is at or ahead of q within the same file.
func (p codePos) beforeOrAt(q codePos) bool {
	if p.line != q.line {
		return p.line < q.line
	}
	return p.col <= q.col
}

// blockIndex holds every recorded control-flow region of a file, from every
// function declared in it.
//
// Keyed by FILE rather than by function on purpose: a function literal has no
// Function record of its own, so its regions are recorded inside the enclosing
// declaration, and a caller identified as `pkg.FuncLit:pos` could not look
// itself up. Positions are absolute, so containment answers the question
// without needing to know which declaration a statement belongs to.
type blockIndex struct {
	byFile map[string][]metadata.Block
}

// newBlockIndex collects the blocks of every function into per-file buckets.
func newBlockIndex(meta *metadata.Metadata) *blockIndex {
	idx := &blockIndex{byFile: map[string][]metadata.Block{}}
	if meta == nil {
		return idx
	}
	for _, pkgName := range meta.SortedPackageNames() {
		pkg := meta.Packages[pkgName]
		if pkg == nil {
			continue
		}
		for _, fileName := range meta.SortedFileNames(pkgName) {
			file := pkg.Files[fileName]
			if file == nil {
				continue
			}
			for _, fnName := range sortedFunctionNames(file) {
				fn := file.Functions[fnName]
				if fn == nil || len(fn.Blocks) == 0 {
					continue
				}
				at := fileOfPosition(meta.StringPool.GetString(fn.Position))
				if at == "" {
					continue
				}
				idx.byFile[at] = append(idx.byFile[at], fn.Blocks...)
			}
		}
	}
	return idx
}

// sortedFunctionNames orders a file's functions so the index is built the same
// way on every run. Nothing here reaches the spec by iteration order — the
// queries are conjunctions over the matching blocks — but a map range that can
// grow into an output path is the bug golden rule #1 exists for.
func sortedFunctionNames(file *metadata.File) []string {
	names := make([]string, 0, len(file.Functions))
	for name := range file.Functions {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// contains reports whether a block's range covers the position. Columns are
// compared, not just lines: a one-line `if` shares every line with the code
// around it, which is the case this exists to get right.
func blockContains(b metadata.Block, pos codePos) bool {
	afterStart := pos.line > b.StartLine || (pos.line == b.StartLine && pos.col >= b.StartCol)
	beforeEnd := pos.line < b.EndLine || (pos.line == b.EndLine && pos.col <= b.EndCol)
	return afterStart && beforeEnd
}

// dominates reports whether every execution that reaches later must first have
// passed through earlier: they are in the same file, earlier is written first,
// and no region encloses earlier without also enclosing later.
//
// Conservative in one direction only. Unknown positions, an unindexed file, or
// a pair the block facts cannot separate all answer true, so a caller that used
// to pair them still does.
func (idx *blockIndex) dominates(earlier, later codePos) bool {
	if idx == nil || !earlier.valid() || !later.valid() {
		return true
	}
	if earlier.file != later.file {
		// Different files means different functions, and pairing is per
		// invocation, so this is not a case the caller can produce. Answer
		// permissively rather than inventing a rule for it.
		return true
	}
	if !earlier.beforeOrAt(later) {
		return false
	}
	for _, b := range idx.byFile[earlier.file] {
		if blockContains(b, earlier) && !blockContains(b, later) {
			// earlier is inside a region later escaped: an `if` arm, a loop, a
			// switch case. Reaching later does not imply earlier ran.
			return false
		}
	}
	return true
}

// exclusive reports whether two positions sit in arms of one conditional and so
// can never both run.
//
// It is what tells `if full { A } else { B }` — two bodies under one status
// write, of which exactly one is sent — apart from `A; …; B`, two bodies on the
// same path where the second is a different response entirely. Both are
// dominated by the write; only the first pair are alternatives (issue #391).
//
// Conservative in the direction that preserves behaviour: unknown positions or
// an unindexed file answer false, so nothing is treated as an alternative on
// missing facts.
func (idx *blockIndex) exclusive(a, b codePos) bool {
	if idx == nil || !a.valid() || !b.valid() || a.file != b.file {
		return false
	}
	blocks := idx.byFile[a.file]
	for i, armA := range blocks {
		if armA.Group == 0 || !blockContains(armA, a) || blockContains(armA, b) {
			continue
		}
		// a is in an arm b is outside of. Two ways that makes them exclusive:
		for j, armB := range blocks {
			// b is in a sibling arm of the same conditional.
			if i != j && armB.Group == armA.Group && blockContains(armB, b) {
				return true
			}
		}
		// Or a's arm exits, so nothing after that conditional runs when a did.
		// This is the early-return idiom — `if bad { …; return }` above the
		// success path — which is how most Go handlers are written and would
		// otherwise read as sequential code.
		if armA.Terminates && afterBlock(armA, b) {
			return true
		}
	}
	return false
}

// afterBlock reports whether a position lies beyond a block's end.
func afterBlock(b metadata.Block, pos codePos) bool {
	return pos.line > b.EndLine || (pos.line == b.EndLine && pos.col > b.EndCol)
}

// controlFlow returns the extractor's block index, built on first use.
func (e *Extractor) controlFlow() *blockIndex {
	if e.blocks == nil {
		e.blocks = newBlockIndex(e.metadata())
	}
	return e.blocks
}
