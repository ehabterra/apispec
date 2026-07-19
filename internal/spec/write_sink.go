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

import "github.com/ehabterra/apispec/internal/metadata"

// unwrapWriteSink resolves a write-sink byte argument back to the payload of the
// serialization transform that produced it. The write sink w.Write(b) sees only
// b's []byte type; when b came from `b, _ := json.Marshal(v)` the response body
// is v's type, one transform hop back. Returns the payload argument (v) when the
// sink's argument traces to a configured BodyTransform, or nil when it does not —
// a raw write (w.Write([]byte("ok"))) has no transform behind it and is kept
// as-is (honest over wrong).
//
// This is the mechanism that lets response detection be anchored on the write
// sink rather than on the marshal call in isolation (issue #195): a json.Marshal
// whose result is never written to a response writer (a downstream client's
// outbound request) is simply never reached from a sink, so it never becomes a
// response.
func (r *ResponsePatternMatcherImpl) unwrapWriteSink(arg *metadata.CallArgument, edge *metadata.CallGraphEdge) *metadata.CallArgument {
	if arg == nil || edge == nil || len(r.cfg.Framework.ResponseContext.BodyTransforms) == 0 {
		return nil
	}
	// Strip address-of/deref/paren so &b and *b trace the same as b.
	for arg != nil && (arg.GetKind() == metadata.KindUnary || arg.GetKind() == metadata.KindStar || arg.GetKind() == metadata.KindParen) {
		arg = arg.X
	}
	if arg == nil {
		return nil
	}

	// Direct local assignment: b, _ := json.Marshal(v); w.Write(b). The
	// assignment records the transform's callee (func + pkg) and its RHS call,
	// whose payload argument is the response value.
	if arg.GetKind() == metadata.KindIdent {
		assigns := r.sinkAssignments(edge, arg.GetName())
		if len(assigns) > 0 {
			a := assigns[len(assigns)-1]
			if idx, ok := r.matchBodyTransform(a.CalleeFunc, a.CalleePkg); ok {
				if a.Value.GetKind() == metadata.KindCall && len(a.Value.Args) > idx {
					return a.Value.Args[idx]
				}
			}
		}
	}

	// Helper-return hop: w.Write(encode(v)) where encode returns a transform
	// result (`b, _ := json.Marshal(e); return b`). The payload is the helper's
	// parameter bound to THIS call's argument, so the type resolves in the
	// caller's scope from the actual call-site value.
	if arg.GetKind() == metadata.KindCall {
		return r.unwrapHelperReturn(arg, edge, 0)
	}

	return nil
}

// unwrapHelperReturn resolves w.Write(helper(a0, a1, …)) to the call-site
// argument that the helper serializes and returns. It finds the helper's return
// value, follows it to a body transform (a returned marshal call, or a local
// `b := json.Marshal(param)`), and — when the transform's payload is one of the
// helper's parameters — binds that parameter back to the matching call-site
// argument (per-call-site correct, unlike a call-graph origin trace that fixes
// on one arbitrary caller). depth bounds recursion through helper-returning
// helpers. Returns nil when the helper doesn't serialize a parameter (honest
// over wrong: a raw-bytes helper produces no body).
func (r *ResponsePatternMatcherImpl) unwrapHelperReturn(call *metadata.CallArgument, edge *metadata.CallGraphEdge, depth int) *metadata.CallArgument {
	if call == nil || call.Fun == nil || depth > 3 {
		return nil
	}
	name := calleeNameOf(call.Fun)
	if name == "" {
		return nil
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return nil
	}
	pkg := call.Fun.GetPkg()
	if pkg == "" {
		pkg = r.contextProvider.GetString(edge.Caller.Pkg)
	}
	fn := findFunctionByName(impl.meta, pkg, name)
	if fn == nil {
		return nil
	}

	// Find the parameter name the helper serializes and returns.
	paramName := r.helperSerializedParam(fn)
	if paramName == "" {
		return nil
	}
	// Bind the parameter to this call's positional argument.
	if i := paramIndexOf(fn, paramName); i >= 0 && i < len(call.Args) {
		return call.Args[i]
	}
	return nil
}

// helperSerializedParam returns the name of the parameter that fn serializes via
// a body transform and returns, or "" when fn does not return a serialized
// parameter. It inspects each returned value: a returned transform call
// (`return json.Marshal(p)`), or a returned local whose assignment is a
// transform (`b, _ := json.Marshal(p); return b`).
func (r *ResponsePatternMatcherImpl) helperSerializedParam(fn *metadata.Function) string {
	consider := func(rv *metadata.CallArgument) string {
		if rv == nil {
			return ""
		}
		// return json.Marshal(p)
		if rv.GetKind() == metadata.KindCall && rv.Fun != nil {
			if idx, ok := r.matchBodyTransform(calleeNameOf(rv.Fun), rv.Fun.GetPkg()); ok && len(rv.Args) > idx {
				if p := rv.Args[idx]; p.GetKind() == metadata.KindIdent {
					return p.GetName()
				}
			}
			return ""
		}
		// return b, where b, _ := json.Marshal(p)
		if rv.GetKind() == metadata.KindIdent {
			for _, a := range fn.AssignmentMap[rv.GetName()] {
				if idx, ok := r.matchBodyTransform(a.CalleeFunc, a.CalleePkg); ok {
					if a.Value.GetKind() == metadata.KindCall && len(a.Value.Args) > idx {
						if p := a.Value.Args[idx]; p.GetKind() == metadata.KindIdent {
							return p.GetName()
						}
					}
				}
			}
		}
		return ""
	}
	for i := range fn.Returns {
		for j := range fn.Returns[i] {
			if p := consider(&fn.Returns[i][j]); p != "" {
				return p
			}
		}
	}
	for i := range fn.ReturnVars {
		if p := consider(&fn.ReturnVars[i]); p != "" {
			return p
		}
	}
	return ""
}

// paramIndexOf returns the positional index of the named parameter in fn's
// signature, or -1. The signature's Args are the parameters in declared order,
// each carrying its declared name (handleFuncType).
func paramIndexOf(fn *metadata.Function, paramName string) int {
	for i, p := range fn.Signature.Args {
		if p != nil && p.GetName() == paramName {
			return i
		}
	}
	return -1
}

// sinkAssignments returns the assignments to `name` visible at the write sink.
// It extends the canonical assignmentsAt with a method-handler case: a plain
// method (not a closure) is absent from file.Functions and has no
// ParentFunction — that field is populated only for function literals — so
// assignmentsAt cannot reach its body. The write sink resolves it via the
// method table keyed by the caller's own receiver type. Scoped to the sink path
// so other resolvers keep their exact behaviour.
func (r *ResponsePatternMatcherImpl) sinkAssignments(edge *metadata.CallGraphEdge, name string) []metadata.Assignment {
	if a := assignmentsAt(r.contextProvider, edge, name); len(a) > 0 {
		return a
	}
	impl, ok := r.contextProvider.(*ContextProviderImpl)
	if !ok || impl.meta == nil {
		return nil
	}
	recv := r.contextProvider.GetString(edge.Caller.RecvType)
	if recv == "" {
		return nil
	}
	am := methodAssignmentMap(impl.meta,
		r.contextProvider.GetString(edge.Caller.Pkg), recv,
		r.contextProvider.GetString(edge.Caller.Name), name)
	return am[name]
}

// matchBodyTransform reports whether a call to (calleeFunc, calleePkg) is a
// configured serialization transform, returning the index of its payload
// argument. An empty PkgRegex matches any package.
func (r *ResponsePatternMatcherImpl) matchBodyTransform(calleeFunc, calleePkg string) (int, bool) {
	if calleeFunc == "" {
		return 0, false
	}
	for _, t := range r.cfg.Framework.ResponseContext.BodyTransforms {
		if t.CallRegex != "" {
			re, err := cachedRegex(t.CallRegex)
			if err != nil || !re.MatchString(calleeFunc) {
				continue
			}
		}
		if t.PkgRegex != "" {
			re, err := cachedRegex(t.PkgRegex)
			if err != nil || !re.MatchString(calleePkg) {
				continue
			}
		}
		return t.ArgIndex, true
	}
	return 0, false
}
