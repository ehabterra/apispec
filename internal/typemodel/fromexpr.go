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

package typemodel

import (
	"go/ast"
	"go/types"
)

// FromExpr builds a TypeRef from an AST type expression at the go/types
// boundary — the structured counterpart of the legacy string stringifier
// (internal/metadata's getTypeName). types.Info resolves selector package
// qualifiers to import paths; it may be nil, in which case the selector's
// identifier text is used as the qualifier.
//
// Two legacy gaps are fixed here because the structured form can carry what a
// flat string dropped: multi-argument generic instantiations (IndexListExpr,
// which getTypeName didn't handle at all) and array lengths (getTypeName
// rendered every array as a slice). Callers migrating from getTypeName should
// use String() for the dotted rendering.
func FromExpr(e ast.Expr, info *types.Info) *TypeRef {
	switch t := e.(type) {
	case nil:
		return &TypeRef{}
	case *ast.Ident:
		return &TypeRef{Kind: KindNamed, Name: t.Name}
	case *ast.StarExpr:
		return &TypeRef{Kind: KindPointer, Elem: FromExpr(t.X, info)}
	case *ast.SelectorExpr:
		return fromSelector(t, info)
	case *ast.IndexExpr:
		base := FromExpr(t.X, info)
		base.Args = append(base.Args, FromExpr(t.Index, info))
		return base
	case *ast.IndexListExpr:
		base := FromExpr(t.X, info)
		for _, idx := range t.Indices {
			base.Args = append(base.Args, FromExpr(idx, info))
		}
		return base
	case *ast.ArrayType:
		r := &TypeRef{Kind: KindSlice, Elem: FromExpr(t.Elt, info)}
		if t.Len != nil {
			r.Kind = KindArray
			r.Len = types.ExprString(t.Len)
		}
		return r
	case *ast.MapType:
		return &TypeRef{Kind: KindMap, Key: FromExpr(t.Key, info), Elem: FromExpr(t.Value, info)}
	case *ast.ChanType:
		r := &TypeRef{Kind: KindChan, Elem: FromExpr(t.Value, info)}
		switch t.Dir {
		case ast.SEND:
			r.Dir = SendOnly
		case ast.RECV:
			r.Dir = RecvOnly
		}
		return r
	case *ast.InterfaceType:
		// Interface literals collapse to the empty interface, matching the
		// legacy stringifier; a named interface arrives as an Ident/Selector.
		return &TypeRef{Kind: KindNamed, Name: "interface{}"}
	case *ast.StructType:
		// Placeholder, matching legacy: the structure itself is captured
		// separately (Field.NestedType).
		return &TypeRef{Kind: KindNamed, Name: "struct{}"}
	case *ast.FuncType:
		return &TypeRef{Kind: KindNamed, Name: "func"}
	case *ast.ParenExpr:
		return FromExpr(t.X, info)
	case *ast.Ellipsis:
		return &TypeRef{Kind: KindSlice, Elem: FromExpr(t.Elt, info)}
	}
	// Opaque fallback: preserve the expression text verbatim.
	return &TypeRef{Kind: KindNamed, Name: types.ExprString(e)}
}

// fromSelector resolves pkgident.Type to an import-path-qualified named ref,
// mirroring the legacy resolution order: package object → any object's name →
// the identifier text.
func fromSelector(sel *ast.SelectorExpr, info *types.Info) *TypeRef {
	r := &TypeRef{Kind: KindNamed, Name: sel.Sel.Name}
	x, ok := sel.X.(*ast.Ident)
	if !ok {
		return r
	}
	r.Pkg = x.Name
	if info != nil {
		if obj := info.ObjectOf(x); obj != nil {
			if pkg, ok := obj.(*types.PkgName); ok {
				r.Pkg = pkg.Imported().Path()
			} else {
				r.Pkg = obj.Name()
			}
		}
	}
	return r
}
