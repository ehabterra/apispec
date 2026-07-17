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
// value was written to a known sink (a bytes.Buffer, a hash, a log) rather than
// to the HTTP response.
//
// It is deliberately PERMISSIVE ("honest over wrong", golden rule #7). The
// destination is first resolved through the call graph to its concrete value
// (see ResponsePatternMatcherImpl.destination), then rejected ONLY when that
// value's type matches a configured known-sink pattern. A destination that is
// the response writer, a custom writer type, an interface (io.Writer), or one
// that cannot be resolved is kept — the gate never drops a real response merely
// because it could not prove the destination is a writer.
type responseDestResolver struct {
	contextProvider ContextProvider
	excludeREs      []*regexp.Regexp // types that are provably NOT the response
}

// newResponseDestResolver compiles the configured regexes once. Enabled()
// reports false when no exclude patterns are configured; callers then fall back
// to prior (fully permissive) behaviour.
func newResponseDestResolver(cfg *APISpecConfig, contextProvider ContextProvider) *responseDestResolver {
	r := &responseDestResolver{contextProvider: contextProvider}
	if cfg == nil {
		return r
	}
	for _, p := range cfg.Framework.ResponseContext.WriterExcludeTypeRegexes {
		if re, err := cachedRegex(p); err == nil {
			r.excludeREs = append(r.excludeREs, re)
		}
	}
	return r
}

// Enabled reports whether any exclude patterns are configured. When false, the
// resolver is skipped and matchers keep their prior behaviour.
func (r *responseDestResolver) Enabled() bool {
	return r != nil && len(r.excludeREs) > 0
}

// IsProvablyNonWriter reports whether the destination argument resolves to a
// type that is a configured known sink and therefore definitely NOT the HTTP
// response. It returns false — permissive — whenever the destination cannot be
// resolved to a type or that type is not on the exclude list, so a proven
// writer, a custom writer, an io.Writer interface, or an unknown destination
// are all kept.
func (r *responseDestResolver) IsProvablyNonWriter(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) bool {
	if !r.Enabled() {
		return false
	}
	t := r.leafType(arg, edge)
	if t == "" {
		return false // unresolved — stay permissive
	}
	return matchAny(r.excludeREs, t)
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
			// Prefer the resolved (concrete/instantiated) type so a traced
			// generic or wrapper argument keeps its real type for gating.
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
