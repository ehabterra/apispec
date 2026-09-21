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
	"github.com/ehabterra/apispec/internal/typemodel"
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
	// The base must BE the receiver, checked by type IDENTITY — package path
	// included. Without the check at all, any identifier selector entered the
	// walk: an unrelated `other.pattern` inside a builder method would resolve
	// `pattern` from the builder's constructor and fabricate a route from a
	// value that has nothing to do with it. Without the PACKAGE, `one.Combo`
	// and `two.Combo` are both "Combo" — the bare-name collision that put a
	// migration's throwaway struct into a real schema in #457, which is not a
	// mistake worth repeating here.
	//
	// By type rather than by variable name, because the receiver's name is not
	// recorded — CalleeRecvVarName is the receiver expression at the CALL site
	// and is empty for a chained call, so comparing names would decline every
	// case this rung exists for. The base's own type is recorded, fully
	// qualified, and the invocation records the receiver's package and type.
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
	if !receiverTypeMatches(arg.X.GetType(), recvPkg, recvName) {
		return "", false
	}
	for e, hops := inv.ChainParent, 0; e != nil && hops < maxChainHops; e, hops = e.ChainParent, hops+1 {
		if value, ok := b.fieldFromReturnedLiteral(e, field, recvPkg, recvName); ok {
			return value, true
		}
	}
	// A chain reached through a VARIABLE has no ChainParent: the receiver was
	// produced by an assignment rather than by the expression the call is
	// written in, so the walk above has nothing to walk (issue #506).
	return b.fieldFromReceiverVar(parent, inv, field, recvPkg, recvName)
}

// fieldFromReceiverVar resolves the field through the assignment that gave the
// RECEIVER VARIABLE its value, for the half of the builder shape that is not
// written as one expression:
//
//	vc := r.Combo("/items")
//	vc.Get(list)
//	vc.Post(create)
//
// which is how a builder is written the moment more than one line hangs off it.
// `r.Combo("/items").Get(list)` links the verb to the constructor through
// ChainParent; assigning it first replaces that link with an assignment, and
// the rung above resolved nothing — the route was dropped, and only reported
// because #428's floor caught the unresolved registration.
//
// This is golden rule #11's shape: a value that reaches a callee by a route
// other than tree containment needs its own binding. The binding here is the
// receiver variable the invocation records (CalleeVarName — despite the name,
// CalleeRecvVarName holds the variable a call's RESULT is assigned to), which
// is tighter evidence than the chain walk's type check: it is the variable
// whose method is being called, not merely something of the same type.
//
// No call-site→edge index is needed for it, which is what the first diagnosis
// of #506 assumed. An assignment's value is a CallArgument, and a CallArgument
// of a call carries its own ParamArgMap — the constructor's `pattern` parameter
// is bound to `"/items"` right there.
//
// Every assignment that can reach the invocation must agree, which is the rule
// and the lookup variableValue uses for a path held in a variable: reading a
// path is the question two different values answer AMBIGUOUSLY, so the
// all-assignments scope is read rather than assignmentsAt's latest-write fast
// path, and writes that cannot reach the call are dropped first (#431, #436).
//
// The site those writes are measured against is the INVOCATION's, not the
// registration's: the variable lives in the caller, so `vb := r.Combo("/b")`
// written below `va.Get(h)` is a write that call never sees, while the
// registration itself sits in another function entirely.
func (b *BasePatternMatcher) fieldFromReceiverVar(invNode TrackerNodeInterface, inv *metadata.CallGraphEdge, field, typePkg, typeName string) (string, bool) {
	if inv == nil || inv.CalleeVarName == "" {
		return "", false
	}
	assigns := pathVarAssignments(b.contextProvider, inv, inv.CalleeVarName)
	assigns = b.assignmentsReaching(assigns, invNode)
	if len(assigns) == 0 {
		return "", false
	}
	value, found := "", false
	for i := range assigns {
		v, ok := b.fieldFromCallChain(&assigns[i].Value, field, typePkg, typeName)
		if !ok {
			return "", false
		}
		if found && v != value {
			return "", false
		}
		value, found = v, true
	}
	return value, found
}

// fieldFromCallChain reads the field out of the first call in a written call
// expression that returns a literal of the type, walking back along the
// receiver the same way the ChainParent walk does — `v := r.Combo("/x").Get(h)`
// assigns the VERB's call, and the verb returns its receiver rather than a
// literal, so the constructor is one hop further in.
func (b *BasePatternMatcher) fieldFromCallChain(call *metadata.CallArgument, field, typePkg, typeName string) (string, bool) {
	for hops := 0; call != nil && hops < maxChainHops; hops++ {
		if call.GetKind() != metadata.KindCall {
			return "", false
		}
		if value, ok := b.fieldFromCall(call, field, typePkg, typeName); ok {
			return value, true
		}
		if call.Fun == nil || call.Fun.GetKind() != metadata.KindSelector {
			return "", false
		}
		call = call.Fun.X
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
	if edge == nil {
		return "", false
	}
	return b.fieldInReturns(b.calleeReturnVars(edge), edge.ParamArgMap, field, typePkg, typeName)
}

// fieldFromCall is fieldFromReturnedLiteral for a call read as an EXPRESSION
// rather than as a call-graph edge — an assignment's right-hand side. The two
// carry the same two facts: what the callee returns, and what this call site
// bound its parameters to.
func (b *BasePatternMatcher) fieldFromCall(call *metadata.CallArgument, field, typePkg, typeName string) (string, bool) {
	if call == nil {
		return "", false
	}
	return b.fieldInReturns(b.callReturnVars(call), call.ParamArgMap, field, typePkg, typeName)
}

// fieldInReturns reads `field` out of the composite literal among `returns`,
// resolving the element through `paramArgMap` when the literal stores one of
// the call's parameters.
func (b *BasePatternMatcher) fieldInReturns(returns []metadata.CallArgument, paramArgMap map[string]metadata.CallArgument, field, typePkg, typeName string) (string, bool) {
	meta := b.metadata()
	for _, ret := range returns {
		ret := ret
		lit := unwrapComposite(&ret)
		if lit == nil || lit.GetKind() != metadata.KindCompositeLit {
			continue
		}
		if !literalConstructs(lit, typePkg, typeName) {
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
			if bound, exists := paramArgMap[elt.GetName()]; exists {
				if value, ok := b.contextProvider.ConstantValue(&bound); ok {
					return value, true
				}
			}
		}
	}
	return "", false
}

// literalConstructs reports whether a composite literal builds the named type,
// read off the literal's own type expression.
//
// A positional literal is read by FIELD INDEX, so a literal of some other
// struct that happens to have an element at that index answers with a value
// that has nothing to do with the receiver. The literal states which type it
// builds; checking it is what keeps the index meaningful.
//
// Unverifiable is not disqualifying: a literal whose type expression records no
// name (a generic instantiation, an inline struct) is left to the field-index
// match, which is the behaviour this check was added around.
func literalConstructs(lit *metadata.CallArgument, typePkg, typeName string) bool {
	if lit == nil || lit.X == nil {
		return true
	}
	name := lit.X.GetName()
	if name == "" || lit.X.GetKind() != metadata.KindIdent {
		return true
	}
	if name != typeName {
		return false
	}
	// Same name in another package is the #457 collision; an unrecorded package
	// cannot be shown to collide, so the name alone decides.
	if pkg := lit.X.GetPkg(); pkg != "" && typePkg != "" {
		return pkg == typePkg
	}
	return true
}

// callReturnVars is calleeReturnVars for a call read as an expression: the
// callee is named by the call's Fun, and a method is found on the type of its
// receiver rather than on a recorded RecvType.
//
// The name comes from calleeNameOf, never from Fun.GetName() alone — a
// cross-package constructor (`web.NewCombo(...)`) has a selector Fun whose name
// lives in .Sel, and reading the ident name would resolve nothing for every
// call that crosses a package (golden rule #10).
func (b *BasePatternMatcher) callReturnVars(call *metadata.CallArgument) []metadata.CallArgument {
	meta := b.metadata()
	if meta == nil || call == nil || call.Fun == nil {
		return nil
	}
	name := calleeNameOf(call.Fun)
	if name == "" {
		return nil
	}
	// `r.Combo(...)`: the receiver's own type says which declaration to read,
	// and a method is not in the file's function table at all.
	if call.Fun.GetKind() == metadata.KindSelector && call.Fun.X != nil {
		if core := typemodel.Parse(call.Fun.X.GetType()).Core(); core != nil && core.Name != "" {
			typ := typeByName(core.Pkg, core.Name, meta)
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
	}
	// A plain function (`NewCombo("/items")`), same package or imported: for a
	// selector the package is on the qualifier, which is the package ident.
	pkg := call.Fun.GetPkg()
	if call.Fun.GetKind() == metadata.KindSelector && call.Fun.X != nil {
		if qualifier := call.Fun.X.GetPkg(); qualifier != "" {
			pkg = qualifier
		}
	}
	if fn := findFunctionByName(meta, pkg, name); fn != nil {
		return fn.ReturnVars
	}
	return nil
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

// receiverTypeMatches reports whether baseType is the same named type as the
// receiver (recvPkg, recvName), comparing package path as well as name.
//
// An unqualified baseType never matches: it cannot be shown to be the
// receiver's type, and resolving from the wrong builder is worse than resolving
// nothing (golden rule #7).
func receiverTypeMatches(baseType, recvPkg, recvName string) bool {
	if baseType == "" || recvPkg == "" || recvName == "" {
		return false
	}
	core := typemodel.Parse(baseType).Core()
	if core == nil || core.Pkg == "" {
		return false
	}
	return core.Pkg == recvPkg && core.Name == recvName
}
