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

// isSerializerCall reports whether this matched call is a configured
// serializer — a call that takes a value and returns bytes, handed no
// destination at all.
//
// Recognised POSITIVELY, by the ResponseContext.BodyTransforms table the sink
// side already uses to trace bytes back to their payload, rather than by the
// absence of a destination field on the pattern. The absence proves nothing:
// net/http's own `^Write$` reads its body from an argument and names no
// destination field either — its writer is simply the receiver — so excluding
// by shape classified the sink itself as a serializer and dropped the response
// it was carrying. A receiver is not reliably recoverable for an interface
// method call, so there is nothing to exclude it by.
//
// Naming the serializers has the same virtue it has on the sink side: the table
// is config, so a project that serializes some other way says so once, and the
// rule stays declarative.
func (r *ResponsePatternMatcherImpl) isSerializerCall(edge *metadata.CallGraphEdge) bool {
	if !r.pattern.TypeFromArg || r.pattern.DestFromReceiver || r.pattern.DestFromAnyArg {
		return false
	}
	if edge == nil || r.destResolver == nil || !r.destResolver.Enabled() {
		return false
	}
	_, ok := r.matchBodyTransform(
		r.contextProvider.GetString(edge.Callee.Name),
		r.contextProvider.GetString(edge.Callee.Pkg),
	)
	return ok
}

// resultReachesWriter reports whether the bytes a serializer returned are
// written to the response writer, within the function the call is written in.
//
// This is the forward question, and it is the only one that separates the two
// shapes a bare `^Marshal$` pattern matches:
//
//	b, _ := json.Marshal(v)                       // ← identical here
//	w.Write(b)                                    // the response
//
//	b, _ := json.Marshal(payload)                 // ← and here
//	http.NewRequest(m, u, bytes.NewReader(b))     // an OUTBOUND request body
//
// Backwards, both are "bytes that came from Marshal". The shipped default never
// had to ask: it anchors on the write sink and traces back through
// ResponseContext.BodyTransforms, so a marshal no sink reaches is simply never
// found (issue #195). A pattern the USER writes gets none of that, and on one
// real service that difference documented a mail provider's payload struct,
// an ERP credential struct and context.Context as responses of the operations
// that happened to reach them (issue #519).
//
// Scoped to the enclosing function and order-insensitive, for the reasons
// bufferReachesWriter sets out: a value handed to another function is that
// function's business, and a `w.Write(b)` textually above the marshal is still
// the same bytes reaching the same writer.
func (r *ResponsePatternMatcherImpl) resultReachesWriter(edge *metadata.CallGraphEdge) bool {
	if edge == nil || r.destResolver == nil || !r.destResolver.Enabled() {
		// No writer types configured: the gate is off, as everywhere else.
		return true
	}
	meta := r.destResolver.metadata()
	if meta == nil {
		return true
	}

	// The variables this call's result was assigned to. An assignment records
	// the callee it came from, which is how the sink side already recognises a
	// transform (unwrapWriteSink).
	names := r.resultVarNames(edge)
	if len(names) == 0 {
		// Not assigned to anything this function names — inlined into another
		// call, or discarded. Nothing to follow, and nothing claimed.
		return false
	}

	for _, sibling := range meta.Callers[edge.Caller.BaseID()] {
		if sibling == nil || sibling == edge {
			continue
		}
		if !callNamesAnyVar(sibling, names) {
			continue
		}
		// The bytes go INTO this call. Whether that call is the wire is the
		// same question the destination gate answers one level out, so it is
		// asked in the same words: the receiver it is called on, or any
		// argument that could be the writer.
		if recv := receiverIdent(meta, sibling); recv != nil &&
			r.destResolver.reachesWriter(recv, sibling, make(map[string]bool, 4)) {
			return true
		}
		for _, arg := range sibling.Args {
			// Provenance only. AnyArgReachesWriter treats an unresolved
			// argument as possibly-the-writer, which is right for a catch-all
			// that must not guess the other way — here it would keep every
			// serializer, since the bytes themselves never resolve to a writer.
			if r.destResolver.reachesWriter(arg, sibling, make(map[string]bool, 4)) {
				return true
			}
		}
	}
	return false
}

// resultVarNames returns the variables the call's result was assigned to in the
// function it is written in, sorted so map iteration cannot reach the answer
// (golden rule #1).
func (r *ResponsePatternMatcherImpl) resultVarNames(edge *metadata.CallGraphEdge) []string {
	calleeName := r.contextProvider.GetString(edge.Callee.Name)
	if calleeName == "" {
		return nil
	}
	calleePkg := r.contextProvider.GetString(edge.Callee.Pkg)

	seen := make(map[string]struct{}, 2)
	var names []string
	collect := func(am map[string][]metadata.Assignment) {
		for name, assigns := range am {
			for _, a := range assigns {
				if a.CalleeFunc != calleeName {
					continue
				}
				// An empty recorded package matches anything: the assignment
				// names the function it came from, and a same-package call
				// records no qualifier.
				if a.CalleePkg != "" && calleePkg != "" && a.CalleePkg != calleePkg {
					continue
				}
				if _, dup := seen[name]; !dup {
					seen[name] = struct{}{}
					names = append(names, name)
				}
				break
			}
		}
	}

	collect(edge.AssignmentMap)
	if impl, ok := r.contextProvider.(*ContextProviderImpl); ok {
		pkg := r.contextProvider.GetString(edge.Caller.Pkg)
		fnName := r.contextProvider.GetString(edge.Caller.Name)
		if fn := findFunctionByName(impl.meta, pkg, fnName); fn != nil {
			collect(fn.AssignmentMap)
		}
		// The enclosing scope may be a METHOD, which lives in Type.Methods
		// rather than the file's function table.
		if recv := r.contextProvider.GetString(edge.Caller.RecvType); recv != "" {
			collect(methodAssignmentsOf(impl.meta, pkg, recv, fnName))
		}
	}
	sort.Strings(names)
	return names
}

// methodAssignmentsOf returns a method's whole AssignmentMap. methodAssignmentMap
// answers for one variable; the forward trace does not know the name yet.
func methodAssignmentsOf(meta *metadata.Metadata, pkg, recv, name string) map[string][]metadata.Assignment {
	if meta == nil || name == "" {
		return nil
	}
	for _, fileName := range meta.SortedFileNames(pkg) {
		for _, t := range meta.SortedTypes(pkg, fileName) {
			for i := range t.Methods {
				m := &t.Methods[i]
				if meta.StringPool.GetString(m.Name) != name {
					continue
				}
				if recv != "" && meta.StringPool.GetString(m.Receiver) != recv {
					continue
				}
				return m.AssignmentMap
			}
		}
	}
	return nil
}

// callNamesAnyVar reports whether any of the call's arguments is one of the
// named variables.
func callNamesAnyVar(call *metadata.CallGraphEdge, names []string) bool {
	for _, arg := range call.Args {
		for _, name := range identsIn(arg, wrapperExprDepth) {
			for _, want := range names {
				if name == want {
					return true
				}
			}
		}
	}
	return false
}

// receiverIdent returns the receiver a method call was made on, as an ident, so
// the writer resolver can trace it the way it traces an argument.
func receiverIdent(meta *metadata.Metadata, call *metadata.CallGraphEdge) *metadata.CallArgument {
	if call.ChainParent != nil && len(call.ChainParent.Args) > 0 {
		return call.ChainParent.Args[0]
	}
	if call.CalleeVarName == "" || meta == nil {
		return nil
	}
	arg := metadata.NewCallArgument(meta)
	arg.SetKind(metadata.KindIdent)
	arg.SetName(call.CalleeVarName)
	return arg
}
