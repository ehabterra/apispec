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
// (issue #170). It decides whether an encoder's write destination disqualifies
// the encoded value from being the operation's response — i.e. whether the
// value was written somewhere OTHER than the HTTP response writer (a
// bytes.Buffer, a hash, a log sink).
//
// It is deliberately CONSERVATIVE ("honest over wrong", golden rule #7): it
// rejects a destination only when it can PROVE the destination is a concrete
// non-writer. A proven writer, a writer-compatible interface (io.Writer — the
// ubiquitous `func writeJSON(w io.Writer, v any)` helper shape), or a
// destination it cannot resolve all stay permissive, so a legitimate response
// is never dropped. The narrower risk it accepts is missing a false positive
// that hides behind an interface indirection — preferable to regressing real
// responses.
type responseDestResolver struct {
	contextProvider ContextProvider
	writerTypeREs   []*regexp.Regexp // types that ARE a response writer
	compatibleREs   []*regexp.Regexp // interfaces a response writer satisfies (io.Writer, ...)
}

// newResponseDestResolver compiles the configured regexes once. Enabled()
// reports false when no ResponseContext writer types are configured; callers
// then fall back to prior (fully permissive) behaviour.
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

// Enabled reports whether ResponseContext writer types are configured. When
// false, the resolver is skipped and matchers keep their prior behaviour.
func (r *responseDestResolver) Enabled() bool {
	return r != nil && len(r.writerTypeREs) > 0
}

// IsProvablyNonWriter reports whether the destination argument resolves to a
// concrete type that is definitely NOT the response writer, and so the encoded
// value must not be treated as the response. It returns false — permissive —
// whenever the destination:
//   - is a configured response-writer type, or
//   - is a writer-compatible interface (io.Writer, ...), or
//   - cannot be resolved to a concrete type.
//
// Only a destination that resolves to a specific, non-writer, non-compatible
// type (bytes.Buffer, os.File, a hash, ...) is reported as provably non-writer.
func (r *responseDestResolver) IsProvablyNonWriter(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) bool {
	if !r.Enabled() {
		return false
	}
	t := r.leafType(arg, edge)
	if t == "" {
		return false // unresolved — stay permissive
	}
	if matchAny(r.writerTypeREs, t) {
		return false // it IS a writer
	}
	if matchAny(r.compatibleREs, t) {
		return false // could be the writer (e.g. an io.Writer parameter)
	}
	return true
}

// leafType resolves the destination expression to the type of its underlying
// value. Address-of and deref are stripped (&buf and buf resolve alike); a bare
// ident yields its (possibly traced) type; a selector/call yields its recorded
// result type when available. Returns "" when the type cannot be determined.
func (r *responseDestResolver) leafType(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) string {
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar) {
		arg = arg.X
	}
	if arg == nil {
		return ""
	}
	switch arg.GetKind() {
	case metadata.KindIdent:
		return r.identType(arg, edge)
	case metadata.KindSelector, metadata.KindCall:
		if t := arg.GetResolvedType(); t != "" {
			return t
		}
		return arg.GetType()
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
