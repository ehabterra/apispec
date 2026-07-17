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
	"regexp"

	"github.com/ehabterra/apispec/internal/metadata"
)

// responseDestResolver is the write-side counterpart of bodySourceResolver
// (issue #170). It decides whether an encoder's write destination is the HTTP
// response by PROVENANCE, not by guessing types: the response writer is the
// handler's writer parameter, so an encode is a response only when its
// destination traces — through parameters, local assignments, and struct
// construction — back to a value of a response-writer type.
//
// The two resolvers are deliberately symmetric: request gating traces a
// decoder's *source* bytes to a request body accessor (r.Body); response gating
// traces an encoder's *destination* to the response writer (w).
type responseDestResolver struct {
	contextProvider ContextProvider
	writerTypeREs   []*regexp.Regexp // types that ARE a response writer
	compatibleREs   []*regexp.Regexp // interfaces a response writer satisfies (io.Writer, ...)
}

// newResponseDestResolver compiles the configured regexes once. Enabled()
// reports false when no writer types are configured; callers then fall back to
// prior (fully permissive) behaviour.
func newResponseDestResolver(cfg *APISpecConfig, contextProvider ContextProvider) *responseDestResolver {
	r := &responseDestResolver{contextProvider: contextProvider}
	if cfg == nil {
		return r
	}
	for _, p := range cfg.Framework.ResponseContext.WriterTypeRegexes {
		if re, err := cachedRegex(p); err == nil {
			r.writerTypeREs = append(r.writerTypeREs, re)
		}
	}
	for _, p := range cfg.Framework.ResponseContext.WriterCompatibleTypeRegexes {
		if re, err := cachedRegex(p); err == nil {
			r.compatibleREs = append(r.compatibleREs, re)
		}
	}
	return r
}

// Enabled reports whether writer types are configured. When false, the resolver
// is skipped and matchers keep their prior behaviour.
func (r *responseDestResolver) Enabled() bool {
	return r != nil && len(r.writerTypeREs) > 0
}

// ShouldDrop reports whether an encode to this destination must NOT be treated
// as the operation response. It drops only when the destination resolves to a
// concrete value that does not trace to the response writer — a bytes.Buffer, a
// hash, a log. A destination that reaches the writer (directly, through a
// parameter, an assignment, or struct construction), stays a writer-compatible
// interface (io.Writer), or cannot be resolved to a type is kept ("honest over
// wrong").
func (r *responseDestResolver) ShouldDrop(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) bool {
	if !r.Enabled() {
		return false
	}
	if r.reachesWriter(arg, edge, make(map[string]bool, 4)) {
		return false // provenance reaches the handler's response writer — it's the response
	}
	t := r.leafType(arg, edge, make(map[string]bool, 4))
	if t == "" || matchAny(r.compatibleREs, t) {
		return false // unresolved, or a writer-compatible interface — keep
	}
	return true // resolved to something with no provenance to w — drop
}

// reachesWriter reports whether the destination's provenance includes a value
// of a response-writer type. It follows address-of/deref, local assignments
// (dst := w), parameter boundaries (a helper's io.Writer param → the caller's
// w), and struct construction (&loggingWriter{w} — the writer embedded in a
// wrapper). This is the write-side mirror of bodySourceResolver.check.
func (r *responseDestResolver) reachesWriter(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	if arg == nil || edge == nil {
		return false
	}
	// Strip address-of and deref so &w and *w trace the same as w.
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil {
		return false
	}

	key := arg.ID()
	if visited[key] {
		return false
	}
	visited[key] = true

	switch arg.GetKind() {
	case metadata.KindIdent:
		return r.reachesWriterIdent(arg, edge, visited)

	case metadata.KindCompositeLit, metadata.KindKeyValue:
		// A struct wrapping the writer: &loggingWriter{w} or
		// loggingWriter{ResponseWriter: w}. Any element (or key-value value)
		// that reaches the writer makes the wrapper a writer.
		for _, el := range arg.Args {
			if r.reachesWriter(el, edge, visited) {
				return true
			}
		}
		if arg.X != nil && r.reachesWriter(arg.X, edge, visited) {
			return true
		}
		if arg.Sel != nil && r.reachesWriter(arg.Sel, edge, visited) {
			return true
		}
		return false
	}
	return false
}

// reachesWriterIdent traces an ident to the handler's response-writer
// PARAMETER — not merely to any writer-typed value. A writer type alone is not
// provenance: a locally-constructed httptest.NewRecorder() is writer-typed but
// is not the handler's `w` (CodeRabbit review on PR #181). So a local (an ident
// with an assignment in scope) is judged by its assigned provenance, and only
// an unassigned ident — the handler/helper parameter itself — is accepted on
// its writer type.
func (r *responseDestResolver) reachesWriterIdent(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	name := arg.GetName()
	if name == "" {
		return false
	}

	// Local assignments come FIRST: dst := w, lw := &loggingWriter{w},
	// rec := httptest.NewRecorder(). The local's provenance is its RHS, not its
	// declared type. latestAssignment checks the call edge's map then the
	// enclosing handler function's scope (a destination assigned in the handler
	// body lives on the Function, not on the Encode call edge).
	if rhs := latestAssignment(r.contextProvider, edge, name); rhs != nil {
		if rhs.Meta == nil {
			rhs.Meta = arg.Meta
		}
		return r.reachesWriter(rhs, edge, visited)
	}

	// No local assignment: the ident is a parameter (or free var). The handler's
	// writer parameter is accepted on its writer type — this is where provenance
	// to `w` is seeded.
	if matchAny(r.writerTypeREs, r.identType(arg, edge)) {
		return true
	}

	// Parameter boundary: a helper's writer parameter traced up to the caller's
	// argument. TraceVariableOrigin yields a synthesized ident with the origin's
	// type; a writer type there means the destination is the response writer.
	callerName := r.contextProvider.GetString(edge.Caller.Name)
	if name == callerName {
		return false // guard against pathological self-recursion
	}
	if meta := r.metadata(); meta != nil {
		callerPkg := r.contextProvider.GetString(edge.Caller.Pkg)
		_, _, originArg, _ := metadata.TraceVariableOrigin(name, callerName, callerPkg, meta)
		if originArg != nil && originArg != arg {
			if t := originArg.GetResolvedType(); t != "" && matchAny(r.writerTypeREs, t) {
				return true
			}
			if matchAny(r.writerTypeREs, originArg.GetType()) {
				return true
			}
		}
	}
	return false
}

// leafType resolves the destination expression to the concrete type of its
// underlying value, used to distinguish a resolved concrete non-writer (drop)
// from an unresolved or interface destination (keep). Address-of/deref are
// stripped, and an ident is followed through its latest local assignment so an
// interface-typed variable reassigned to a concrete value (`var d io.Writer =
// w; d = &bytes.Buffer{}`) is classified by that concrete value rather than its
// declared interface type.
func (r *responseDestResolver) leafType(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) string {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil {
		return ""
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		if key := arg.ID(); !visited[key] {
			visited[key] = true
			if rhs := latestAssignment(r.contextProvider, edge, arg.GetName()); rhs != nil {
				if t := r.leafType(rhs, edge, visited); t != "" {
					return t
				}
			}
		}
		return r.identType(arg, edge)
	case metadata.KindSelector, metadata.KindCall, metadata.KindCompositeLit:
		if t := arg.GetResolvedType(); t != "" {
			return t
		}
		if t := arg.GetType(); t != "" {
			return t
		}
		// A composite literal (bytes.Buffer{}) carries its type on arg.X, not on
		// Type/ResolvedType — render it via the context provider.
		return r.contextProvider.GetArgumentInfo(arg)
	}
	return ""
}

// identType returns the ident's declared type, preferring the resolved type.
// Falls back to the concrete type recorded on a local assignment, then to a
// call-graph origin trace. Mirrors bodySourceResolver.identType.
func (r *responseDestResolver) identType(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) string {
	if arg == nil {
		return ""
	}
	if t := arg.GetResolvedType(); t != "" {
		return t
	}
	if t := arg.GetType(); t != "" {
		return t
	}
	if edge != nil && len(edge.AssignmentMap) > 0 {
		if assigns, ok := edge.AssignmentMap[arg.GetName()]; ok {
			for i := len(assigns) - 1; i >= 0; i-- {
				if t := r.contextProvider.GetString(assigns[i].ConcreteType); t != "" {
					return t
				}
			}
		}
	}
	if meta := r.metadata(); meta != nil && edge != nil {
		callerName := r.contextProvider.GetString(edge.Caller.Name)
		callerPkg := r.contextProvider.GetString(edge.Caller.Pkg)
		_, _, originArg, _ := metadata.TraceVariableOrigin(arg.GetName(), callerName, callerPkg, meta)
		if originArg != nil && originArg != arg {
			if t := originArg.GetResolvedType(); t != "" {
				return t
			}
			return originArg.GetType()
		}
	}
	return ""
}

func (r *responseDestResolver) metadata() *metadata.Metadata {
	if impl, ok := r.contextProvider.(*ContextProviderImpl); ok {
		return impl.meta
	}
	return nil
}
