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

// responseDestResolver is the write-side mirror of bodySourceResolver
// (issue #170). It determines whether an encoder's write destination can be
// traced back to the HTTP response writer as configured by
// FrameworkConfig.ResponseContext. This is the "response-destination gating"
// that stops a value encoded to some other io.Writer — a bytes.Buffer, a hash,
// a log sink — from being mistaken for the operation's response.
//
// The two resolvers are deliberately symmetric: request gating traces a
// decoder's *source* bytes to a request-body accessor; response gating traces
// an encoder's *destination* writer to a response writer.
type responseDestResolver struct {
	contextProvider ContextProvider
	writerTypeREs   []*regexp.Regexp
	accessorREs     []*regexp.Regexp
}

// newResponseDestResolver compiles the configured regexes once. Enabled()
// reports false when no ResponseContext writer types are configured; callers
// then fall back to prior (permissive) behaviour.
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
	for _, p := range cfg.Framework.ResponseContext.WriterAccessors {
		if re, err := cachedRegex(p); err == nil {
			r.accessorREs = append(r.accessorREs, re)
		}
	}
	return r
}

// Enabled reports whether ResponseContext writer types are configured. When
// false, the resolver is skipped and matchers keep their prior behaviour.
func (r *responseDestResolver) Enabled() bool {
	return r != nil && len(r.writerTypeREs) > 0
}

// IsResponseDest returns true if the destination argument at the given call
// site can be traced through selectors, idents, assignments and parameter
// boundaries to a response-writer root.
//
// When Enabled() is false it returns true (permissive) so callers can use it
// unconditionally.
func (r *responseDestResolver) IsResponseDest(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) bool {
	if !r.Enabled() {
		return true
	}
	visited := make(map[string]bool, 4)
	return r.check(arg, edge, visited)
}

func (r *responseDestResolver) check(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	if arg == nil || edge == nil {
		return false
	}

	// Strip address-of and deref so &w and *w trace the same as w.
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar) {
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
	case metadata.KindSelector, metadata.KindCall:
		root, segs := peelAccessorChain(arg)
		if root != nil && r.chainMatches(root, segs, edge) {
			return true
		}
		// The root may itself be a variable whose origin is the writer —
		// e.g. dst := w; json.NewEncoder(dst).Encode(v).
		if root != nil && root.GetKind() == metadata.KindIdent {
			return r.checkIdent(root, edge, visited)
		}
		return false

	case metadata.KindIdent:
		return r.checkIdent(arg, edge, visited)
	}
	return false
}

// chainMatches reports whether the (root, segs) chain points at a response
// writer: the root's type must match a WriterTypeRegex when segs is empty
// (the writer IS the root, e.g. net/http's `w`), otherwise the dotted accessor
// chain must match a WriterAccessor applied to a writer-typed context root.
func (r *responseDestResolver) chainMatches(root *metadata.CallArgument, segs []chainSegment, edge *metadata.CallGraphEdge) bool {
	if root == nil || root.GetKind() != metadata.KindIdent {
		return false
	}
	rootType := r.identType(root, edge)
	if rootType == "" {
		return false
	}
	// Bare writer parameter (no accessor chain): the root's own type decides.
	if len(segs) == 0 {
		return matchAny(r.writerTypeREs, rootType)
	}
	// Writer reached through a context accessor (e.g. c.Writer): the accessor
	// chain must match. WriterAccessors is optional; when unset, only bare
	// writer roots qualify.
	return matchAny(r.accessorREs, accessorString(segs))
}

// checkIdent traces an ident through local assignments and parameter
// boundaries to see whether its value is the response writer.
func (r *responseDestResolver) checkIdent(arg *metadata.CallArgument, edge *metadata.CallGraphEdge, visited map[string]bool) bool {
	if arg == nil || arg.GetKind() != metadata.KindIdent || edge == nil {
		return false
	}
	name := arg.GetName()
	if name == "" {
		return false
	}

	// A bare ident whose own type is already a writer qualifies directly —
	// this is the common `json.NewEncoder(w).Encode(v)` case.
	if t := r.identType(arg, edge); t != "" && matchAny(r.writerTypeREs, t) {
		return true
	}

	// 1) Local assignments visible at this call site (dst := w).
	if assigns, ok := edge.AssignmentMap[name]; ok && len(assigns) > 0 {
		rhs := assigns[len(assigns)-1].Value
		if rhs.Meta == nil {
			rhs.Meta = arg.Meta
		}
		if r.check(&rhs, edge, visited) {
			return true
		}
	}

	// 2) The ident may be a parameter of a helper — trace it up the call graph
	// to the caller's argument so writeJSON(w, v) { enc(w) } resolves from its
	// caller passing the real response writer.
	callerName := r.contextProvider.GetString(edge.Caller.Name)
	callerPkg := r.contextProvider.GetString(edge.Caller.Pkg)
	if name == callerName {
		return false // guard against pathological self-recursion
	}
	if meta := r.metadata(); meta != nil {
		_, _, originArg, _ := metadata.TraceVariableOrigin(name, callerName, callerPkg, meta)
		if originArg != nil && originArg != arg {
			if t := originArg.GetType(); t != "" && matchAny(r.writerTypeREs, t) {
				return true
			}
		}
	}

	return false
}

// identType returns the ident's declared type, preferring the resolved type.
// Falls back to local assignments and a call-graph origin trace. Mirrors
// bodySourceResolver.identType.
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
		if originArg != nil {
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
