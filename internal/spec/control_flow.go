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

// Control-flow queries over the block ranges metadata records (Function.Blocks
// and Method.Blocks).
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
// function and method declared in it.
//
// Keyed by FILE rather than by function on purpose: a function literal has no
// Function record of its own, so its regions are recorded inside the enclosing
// declaration, and a caller identified as `pkg.FuncLit:pos` could not look
// itself up. Positions are absolute, so containment answers the question
// without needing to know which declaration a statement belongs to.
//
// What containment cannot answer is any rule that relates two regions, or a
// region and a point outside it — and both exclusivity rules do. So each entry
// keeps its declaration's identity: groups are renumbered to be unique per
// file, and the declaration's line span travels with every block.
type blockIndex struct {
	byFile map[string][]indexedBlock
}

// indexedBlock is a recorded region plus the line span of the declaration it
// was recorded in. Zero span lines mean the span is unknown (no parsable
// Position, or a declaration with no recorded EndLine), and every rule that
// needs the span then answers in its conservative direction.
type indexedBlock struct {
	metadata.Block
	declStart, declEnd int
}

// inDecl reports whether a position lies within the block's declaration. An
// unknown span answers false.
func (b indexedBlock) inDecl(pos codePos) bool {
	return b.declStart > 0 && b.declEnd >= b.declStart &&
		pos.line >= b.declStart && pos.line <= b.declEnd
}

// newBlockIndex collects the blocks of every function and method into
// per-file buckets.
//
// A method is filed under the file its Position names, not the file its type
// is declared in: Type.Methods gathers every method of the package onto the
// type, wherever each is written. A method is listed once, on the type entry of
// the file declaring the type, so nothing is indexed twice.
//
// Block.Group is numbered per declaration, so every file with two
// conditionals in two declarations holds two "group 1"s. The index appends
// copies whose groups are shifted past every group already in the file's
// bucket: left alone, an arm of one handler would read as a sibling arm of a
// conditional in the next, and two bodies that can both run would be paired as
// alternatives. (Block.Parent is left as recorded — it indexes the
// declaration's own slice, and no query here reads it.)
func newBlockIndex(meta *metadata.Metadata) *blockIndex {
	idx := &blockIndex{byFile: map[string][]indexedBlock{}}
	if meta == nil {
		return idx
	}
	groupBase := map[string]int{}
	add := func(position, endLine int, blocks []metadata.Block) {
		if len(blocks) == 0 {
			return
		}
		at, line, _ := parsePosition(meta.StringPool.GetString(position))
		if at == "" {
			return
		}
		if endLine < line {
			// A span that cannot be trusted is recorded as unknown.
			line, endLine = 0, 0
		}
		base, top := groupBase[at], 0
		bucket := idx.byFile[at]
		for _, b := range blocks {
			if b.Group != 0 {
				top = max(top, b.Group)
				b.Group += base
			}
			bucket = append(bucket, indexedBlock{Block: b, declStart: line, declEnd: endLine})
		}
		idx.byFile[at] = bucket
		groupBase[at] = base + top
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
				if fn := file.Functions[fnName]; fn != nil {
					add(fn.Position, fn.EndLine, fn.Blocks)
				}
			}
			for _, typ := range meta.SortedTypes(pkgName, fileName) {
				for i := range typ.Methods {
					// An interface method has no body and so no blocks.
					m := &typ.Methods[i]
					add(m.Position, m.EndLine, m.Blocks)
				}
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
		if b.Kind == metadata.BlockScope {
			continue // a scope is not a branch: the init it opens always runs
		}
		if blockContains(b.Block, earlier) && !blockContains(b.Block, later) {
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
		if armA.Group == 0 || !blockContains(armA.Block, a) || blockContains(armA.Block, b) {
			continue
		}
		// a is in an arm b is outside of. Two ways that makes them exclusive:
		for j, armB := range blocks {
			// b is in a sibling arm of the same conditional. Groups are unique
			// per file in the index, so a sibling is in the same declaration.
			if i != j && armB.Group == armA.Group && blockContains(armB.Block, b) {
				return true
			}
		}
		// Or a's arm exits, so nothing after that conditional runs when a did.
		// This is the early-return idiom — `if bad { …; return }` above the
		// success path — which is how most Go handlers are written and would
		// otherwise read as sequential code.
		//
		// "After" means after it in the SAME function: the arm's return
		// leaves its own function, which says nothing about a function
		// written below it in the file. Without a known span, no answer.
		// The innermost function may be a literal, whose return ends only
		// the literal — code after it in the enclosing declaration still
		// runs once the literal has returned.
		if armA.Terminates && afterBlock(armA.Block, b) && armA.inDecl(b) &&
			!literalEnds(blocks, armA.Block, b) {
			return true
		}
	}
	return false
}

// literalEnds reports whether a function literal encloses the arm but not pos:
// the arm's return then leaves that literal, and pos can still run after it.
// A literal nested INSIDE the arm is not one — the arm's own return is not in
// it.
func literalEnds(blocks []indexedBlock, arm metadata.Block, pos codePos) bool {
	armStart := codePos{line: arm.StartLine, col: arm.StartCol}
	for _, lit := range blocks {
		if lit.Kind == metadata.BlockFuncLit && blockContains(lit.Block, armStart) && !blockContains(lit.Block, pos) {
			return true
		}
	}
	return false
}

// inScope reports whether a name bound at earlier is still visible at later: no
// initializer scope holds earlier without also holding later.
//
// Dominance cannot answer this. An init statement runs unconditionally, so it
// dominates everything after its statement — and the name it declared is gone
// there (`if h := func…; h != nil {}` then `register(h)` names another `h`).
// Missing facts answer true, as dominates does.
func (idx *blockIndex) inScope(earlier, later codePos) bool {
	if idx == nil || !earlier.valid() || !later.valid() || earlier.file != later.file {
		return true
	}
	for _, b := range idx.byFile[earlier.file] {
		if b.Kind == metadata.BlockScope && blockContains(b.Block, earlier) && !blockContains(b.Block, later) {
			return false
		}
	}
	return true
}

// alwaysFollows reports whether later runs whenever earlier does: earlier
// dominates it, and no region holds later without also holding earlier — so
// later sits in no branch, loop or case that earlier's path can skip. Missing
// facts answer true, as dominates does.
func (idx *blockIndex) alwaysFollows(earlier, later codePos) bool {
	if !idx.dominates(earlier, later) {
		return false
	}
	if idx == nil || !earlier.valid() || !later.valid() || earlier.file != later.file {
		return true
	}
	for _, b := range idx.byFile[later.file] {
		if b.Kind != metadata.BlockScope && blockContains(b.Block, later) && !blockContains(b.Block, earlier) {
			return false
		}
	}
	return true
}

// exitsBetween returns the regions written between earlier and later, in the
// function holding both, whose control leaves that function: the early-return
// arms a path from earlier can take without ever reaching later. A region
// holding either position is not one — it is a branch, which alwaysFollows
// already answers, or it ends neither path. nil on missing facts.
func (idx *blockIndex) exitsBetween(earlier, later codePos) []metadata.Block {
	if idx == nil || !earlier.valid() || !later.valid() || earlier.file != later.file {
		return nil
	}
	blocks := idx.byFile[later.file]
	var exits []metadata.Block
	for _, b := range blocks {
		if !b.Terminates || blockContains(b.Block, earlier) || blockContains(b.Block, later) {
			continue
		}
		start := codePos{line: b.StartLine, col: b.StartCol}
		if !start.beforeOrAt(earlier) && afterBlock(b.Block, later) && b.inDecl(earlier) && b.inDecl(later) &&
			!literalEnds(blocks, b.Block, later) {
			exits = append(exits, b.Block)
		}
	}
	return exits
}

// sharesRegion reports whether some recorded region contains both positions.
//
// Used to decide whether a write BELOW a call can still be the value at it: in
// a loop body, code after the call runs before the call's next iteration, so
// "later in the source" is not "cannot reach" (issue #436). Any enclosing
// region counts, not only a loop: an `if` arm holding both is a region entered
// as a unit, and answering yes there keeps the conservative behaviour.
//
// Missing facts answer false, which drops the write — so callers must only ask
// about a position they already know is below the call.
func (idx *blockIndex) sharesRegion(a, b codePos) bool {
	if idx == nil || !a.valid() || !b.valid() || a.file != b.file {
		return false
	}
	for _, block := range idx.byFile[a.file] {
		if block.Kind != metadata.BlockScope && blockContains(block.Block, a) && blockContains(block.Block, b) {
			return true
		}
	}
	return false
}

// afterBlock reports whether a position lies beyond a block's end.
func afterBlock(b metadata.Block, pos codePos) bool {
	return pos.line > b.EndLine || (pos.line == b.EndLine && pos.col > b.EndCol)
}

// controlFlow returns the block index, built on first use.
//
// Held on the context provider rather than per asker: the provider is created
// once per run and handed to every matcher, and building the index walks every
// function of every package — so this is the one place that cost is paid.
func (e *Extractor) controlFlow() *blockIndex {
	if impl, ok := e.contextProvider.(*ContextProviderImpl); ok {
		return impl.controlFlow()
	}
	if e.blocks == nil {
		e.blocks = newBlockIndex(e.metadata())
	}
	return e.blocks
}

// controlFlow returns the provider's block index, built on first use.
func (c *ContextProviderImpl) controlFlow() *blockIndex {
	if c == nil {
		return nil
	}
	if c.blocks == nil {
		c.blocks = newBlockIndex(c.meta)
	}
	return c.blocks
}
