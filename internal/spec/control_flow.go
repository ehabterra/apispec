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

// Mutual exclusion — two positions in arms of one conditional, which
// Block.Group records — is the other question these facts answer, and the one a
// summary of a function's responses needs when it merges the arms of a branch.
// Dominance already rejects every pair the pairing rule cares about, so no
// exclusivity query is written here until that summary needs one.

// controlFlow returns the extractor's block index, built on first use.
func (e *Extractor) controlFlow() *blockIndex {
	if e.blocks == nil {
		e.blocks = newBlockIndex(e.metadata())
	}
	return e.blocks
}
