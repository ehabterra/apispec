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

package metadata

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
	"strings"
)

// positionIndex returns the string-pool index of a source position, formatting it
// at most once per distinct location.
//
// Every call argument, call, function, assignment and struct instance records a
// position, and the same location is recorded many times over — once per node
// that mentions it, and again each time a node is revisited. Formatting per
// record cost 1.03GB, 13.5% of all allocation on a 360-package project (issue
// #226): the string was built with fmt and only then interned, so interning saved
// nothing. Here the pool index is remembered, so a repeat costs one map lookup.
//
// The cache keys on token.Pos, which is a byte offset within the FileSet and so
// identifies a location exactly — no heuristic, and no way for two locations to
// collide.
//
// Returns -1 (the pool's "no string") for a position that does not exist, which
// is what interning the empty string produced before.
func (m *Metadata) positionIndex(pos token.Pos, fset *token.FileSet) int {
	if m == nil || m.StringPool == nil {
		return -1
	}
	if !pos.IsValid() || fset == nil {
		return -1
	}

	m.posMutex.RLock()
	idx, ok := m.posCache[pos]
	m.posMutex.RUnlock()
	if ok {
		return idx
	}

	// A Pos is only meaningful in the FileSet that issued it: for anything else
	// — a Pos from another FileSet, or one past the end of this one —
	// fset.Position returns the zero value, which renders as "-". Pooling that
	// would record a position of "-" on the node, so it is treated as no position
	// at all. The check is here rather than up front because Position already does
	// the file lookup, and a location that resolves pays for it once.
	p := fset.Position(pos)
	if !p.IsValid() {
		return -1
	}

	idx = m.StringPool.Get(formatPosition(p))

	m.posMutex.Lock()
	if m.posCache == nil {
		m.posCache = make(map[token.Pos]int, 4096)
	}
	m.posCache[pos] = idx
	m.posMutex.Unlock()
	return idx
}

// positionString returns the same rendering as positionIndex, for the callers
// that build a name out of it (a function literal is identified by where it is
// written). The string comes from the pool, so a repeat allocates nothing.
func (m *Metadata) positionString(pos token.Pos, fset *token.FileSet) string {
	idx := m.positionIndex(pos, fset)
	if idx < 0 {
		return ""
	}
	return m.StringPool.GetString(idx)
}

// FuncLitPrefix marks a name that identifies a function literal. A closure has no
// declared name, so it is identified by where it is written: `FuncLit:<position>`.
// The name reaches tracker keys and a closure route's operationId, so the three
// sites that mint it all go through funcLitName.
const FuncLitPrefix = "FuncLit:"

// funcLitName builds the identifier for a function literal written at position.
func funcLitName(position string) string {
	return FuncLitPrefix + position
}

// stablePosition renders a position the way a closure is identified by it: the
// same `file:line:column`, but with a filename that does not depend on where the
// checkout lives.
//
// A closure has no declared name, so `FuncLit:<position>` IS its identity — and
// that identity reaches the generated spec as a route's operationId. With the
// absolute filename in it, the same source produced a different spec on every
// machine, so the output could not be diffed, reviewed or committed (issue #216).
// The identity is what gets normalised; the Position recorded on each node stays
// absolute, because that is what a reader needs to open the file.
func (m *Metadata) stablePosition(pos token.Pos, fset *token.FileSet) string {
	if m == nil || !pos.IsValid() || fset == nil {
		return ""
	}
	p := fset.Position(pos)
	if !p.IsValid() {
		return ""
	}
	p.Filename = m.stableFilename(p.Filename)
	return formatPosition(p)
}

// stableFilename maps a source path to a form that is the same in every checkout.
//
// Three cases, in order:
//
//   - inside the analysed module: relative to its root, with forward slashes so
//     the answer does not depend on the OS either;
//   - inside the module cache: the part after `pkg/mod/`, which already carries
//     the module and its version (`github.com/x/y@v1.2.3/f.go`);
//   - anything else: unchanged. A path outside both is left alone rather than
//     rewritten into something that looks stable but is not (the depth of a
//     `../../..` chain depends on where the module sits).
func (m *Metadata) stableFilename(file string) string {
	if file == "" {
		return file
	}
	if m.moduleDir != "" {
		if rel, err := filepath.Rel(m.moduleDir, file); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	if idx := strings.LastIndex(file, modCacheMarker); idx >= 0 {
		return filepath.ToSlash(file[idx+len(modCacheMarker):])
	}
	return file
}

// modCacheMarker is the path segment every module-cache file sits under, whatever
// GOMODCACHE is set to.
const modCacheMarker = "pkg/mod/"

// funcPositionIndex is positionIndex for a function declaration.
func (m *Metadata) funcPositionIndex(fn *ast.FuncDecl, fset *token.FileSet) int {
	if fn == nil {
		return -1
	}
	return m.positionIndex(fn.Pos(), fset)
}

// varPositionIndex is positionIndex for a variable's identifier.
func (m *Metadata) varPositionIndex(ident *ast.Ident, fset *token.FileSet) int {
	if ident == nil {
		return -1
	}
	return m.positionIndex(ident.Pos(), fset)
}

// formatPosition renders a position as `file:line:column`.
//
// This is go/token.Position.String's output, reproduced deliberately rather than
// called: that method builds the result with fmt and string concatenation, and it
// was the single largest allocation site in metadata generation. The behaviour
// must stay identical down to the empty-filename and zero-column cases, because
// positions are part of tracker keys, of a closure's identity, and of the
// byte-compared metadata goldens.
func formatPosition(p token.Position) string {
	if !p.IsValid() {
		if p.Filename == "" {
			return "-"
		}
		return p.Filename
	}

	// file:line:col, sized up front: a path plus two numbers.
	buf := make([]byte, 0, len(p.Filename)+12)
	buf = append(buf, p.Filename...)
	if p.Filename != "" {
		buf = append(buf, ':')
	}
	buf = strconv.AppendInt(buf, int64(p.Line), 10)
	if p.Column != 0 {
		buf = append(buf, ':')
		buf = strconv.AppendInt(buf, int64(p.Column), 10)
	}
	return string(buf)
}
