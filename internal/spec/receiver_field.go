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
	"github.com/ehabterra/apispec/internal/metadata"
)

// receiverFieldValue resolves `c.field` where `c` is the receiver of the method
// the registration is written in, by following the call chain to the call that
// CONSTRUCTED that receiver and reading the field out of the composite literal
// it returns.
//
// This is the builder shape, and gitea has 60 routes of it:
//
//	r.Combo("/items").Get(list).Post(create)
//
// The path is given once, to the constructor, and each verb reads it back off
// the receiver — so the registration itself has no path to read. Before this it
// resolved to nothing: #463 stopped the argument being rendered as the Go symbol
// `gitea.dev/modules/web.Combo.pattern`, which left an honest `{pattern}`
// placeholder; this resolves the value it stands for (issue #461).
//
// Nothing here matches on names, and nothing is guessed:
//
//   - a candidate call qualifies only by RETURNING a composite literal of a type
//     that declares the field, so the value read is the one that call put there;
//   - the chain is walked until such a call is found, because a chained verb
//     (`.Get(...).Post(...)`) links to the previous verb rather than to the
//     constructor, and a verb returns its receiver rather than a literal;
//   - the field's value must resolve to a constant, either directly or through
//     the constructor's own argument for the parameter it names. Anything else
//     stays unresolved, and the caller falls back to the placeholder
//     (golden rule #7).
func (b *BasePatternMatcher) receiverFieldValue(arg *metadata.CallArgument, node TrackerNodeInterface) (string, bool) {
	if arg == nil || arg.GetKind() != metadata.KindSelector || arg.X == nil || arg.Sel == nil {
		return "", false
	}
	field := arg.Sel.GetName()
	if field == "" || arg.X.GetKind() != metadata.KindIdent {
		return "", false
	}
	if node == nil || b.metadata() == nil {
		return "", false
	}
	// The enclosing method's own invocation is the parent frame; its chain
	// parent is the call the receiver came from.
	parent := node.GetParent()
	if parent == nil {
		return "", false
	}
	inv := parent.GetEdge()
	if inv == nil {
		return "", false
	}
	// The field belongs to the RECEIVER's type, which the invocation records —
	// the constructor's literal carries no type of its own. If the constructor
	// had built a different type, the receiver would not have this one.
	recvPkg := b.contextProvider.GetString(inv.Callee.Pkg)
	recvName := bareTypeName(b.contextProvider.GetString(inv.Callee.RecvType))
	if recvName == "" {
		return "", false
	}
	// The base must BE the receiver, checked by type. Without this, any
	// identifier selector entered the walk: an unrelated `other.pattern` inside
	// a builder method would resolve `pattern` from the builder's constructor
	// and fabricate a route from a value that has nothing to do with it.
	//
	// By type rather than by name, because the receiver's variable name is not
	// recorded — CalleeRecvVarName is the receiver expression at the CALL site
	// and is empty for a chained call, so comparing names would decline every
	// case this rung exists for. The base's own type is recorded, and it is the
	// fact that matters.
	//
	// Two limits this leaves, both measured rather than assumed:
	//
	//   - a different value of the SAME builder type passes the check, and is
	//     read from this chain's constructor. Narrowing that needs the
	//     receiver's identity, which metadata does not carry.
	//   - when one registration serves SEVERAL builder chains
	//     (`r.Combo("/alpha").Get(a)` and `r.Combo("/beta").Get(b)` share the
	//     call site inside Get), the extractor collapses them into ONE route
	//     before this rung is asked — the floor reports a single unresolved
	//     registration for the pair, not two — so the value resolved here is
	//     the first chain's, and the others were already lost upstream. That
	//     collapse is #465, not something this rung can see.
	if bareTypeName(arg.X.GetType()) != recvName {
		return "", false
	}
	for e, hops := inv.ChainParent, 0; e != nil && hops < maxChainHops; e, hops = e.ChainParent, hops+1 {
		if value, ok := b.fieldFromReturnedLiteral(e, field, recvPkg, recvName); ok {
			return value, true
		}
	}
	return "", false
}

// maxChainHops bounds the walk back along a method chain. A builder chain is a
// handful of links (`.Get().Post().Patch()`); the bound is what keeps a cyclic
// ChainParent from being a hang rather than a wrong answer.
const maxChainHops = 32

// fieldFromReturnedLiteral reads `field` out of the composite literal that
// edge's callee returns, resolving the element through that call's own
// arguments when the literal stores a parameter.
func (b *BasePatternMatcher) fieldFromReturnedLiteral(edge *metadata.CallGraphEdge, field, typePkg, typeName string) (string, bool) {
	meta := b.metadata()
	for _, ret := range b.calleeReturnVars(edge) {
		ret := ret
		lit := unwrapComposite(&ret)
		if lit == nil || lit.GetKind() != metadata.KindCompositeLit {
			continue
		}
		elt, ok := literalFieldElement(meta, lit, field, typePkg, typeName)
		if !ok {
			continue
		}
		if value, ok := b.contextProvider.ConstantValue(elt); ok {
			return value, true
		}
		// The literal stores one of the constructor's parameters, which is the
		// whole point of a builder: `&Combo{r, pattern}` holds what the caller
		// passed for `pattern`.
		if elt.GetKind() == metadata.KindIdent {
			if bound, exists := edge.ParamArgMap[elt.GetName()]; exists {
				if value, ok := b.contextProvider.ConstantValue(&bound); ok {
					return value, true
				}
			}
		}
	}
	return "", false
}

// calleeReturnVars returns the callee's recorded return values, whether it is a
// method (`(r *Router) Combo`) or a plain function (`NewCombo`).
func (b *BasePatternMatcher) calleeReturnVars(edge *metadata.CallGraphEdge) []metadata.CallArgument {
	meta := b.metadata()
	if meta == nil || edge == nil {
		return nil
	}
	name := b.contextProvider.GetString(edge.Callee.Name)
	pkg := b.contextProvider.GetString(edge.Callee.Pkg)
	if name == "" {
		return nil
	}
	if recv := b.contextProvider.GetString(edge.Callee.RecvType); recv != "" {
		typ := typeByName(pkg, bareTypeName(recv), meta)
		if typ == nil {
			return nil
		}
		for i := range typ.Methods {
			if getStringFromPool(meta, typ.Methods[i].Name) == name {
				return typ.Methods[i].ReturnVars
			}
		}
		return nil
	}
	if fn := findFunctionByName(meta, pkg, name); fn != nil {
		return fn.ReturnVars
	}
	return nil
}

// literalFieldElement returns the element of a composite literal that sets
// `field`, keyed or positional.
//
// A positional literal (`&Combo{r, pattern}`) names no fields, so the element is
// found by matching the index against the STRUCT's field order — which is a fact
// metadata records, not an assumption about argument order. Without that, the
// builder shape resolves nothing: a constructor almost always writes its literal
// positionally.
func literalFieldElement(meta *metadata.Metadata, lit *metadata.CallArgument, field, typePkg, typeName string) (*metadata.CallArgument, bool) {
	keyed := false
	for _, elt := range lit.Args {
		if elt == nil {
			continue
		}
		if elt.GetKind() != metadata.KindKeyValue {
			continue
		}
		keyed = true
		if elt.X != nil && elt.X.GetName() == field && elt.Fun != nil {
			return elt.Fun, true
		}
	}
	if keyed {
		// Keyed and not among the keys: the field is at its zero value, which
		// is not a path. Reported as unresolved rather than as "".
		return nil, false
	}
	idx, ok := structFieldIndex(meta, typePkg, typeName, field)
	if !ok || idx < 0 || idx >= len(lit.Args) {
		return nil, false
	}
	return lit.Args[idx], lit.Args[idx] != nil
}

// structFieldIndex is the declaration index of `field` in the named struct, or
// false when the type or the field cannot be found.
func structFieldIndex(meta *metadata.Metadata, pkg, name, field string) (int, bool) {
	if meta == nil || name == "" {
		return 0, false
	}
	typ := typeByName(pkg, name, meta)
	if typ == nil {
		return 0, false
	}
	for i := range typ.Fields {
		if getStringFromPool(meta, typ.Fields[i].Name) == field {
			return i, true
		}
	}
	return 0, false
}
