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
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

// promotedLib stands in for a framework: Engine embeds Group, so Engine's GET
// is Group's, while Use is Engine's own.
const promotedLib = `package lib

type Group struct{}

func (g *Group) GET(path string) {}

type Engine struct{ Group }

func (e *Engine) Use() {}

type Routes interface{ Mount(path string) }
`

const promotedApp = `package app

import "example.com/lib"

// App embeds the framework engine.
type App struct{ *lib.Engine }

// Shadow declares its own GET, which shadows the promoted one.
type Shadow struct{ *lib.Engine }

func (s Shadow) GET(path string) {}

// Base is generic; Ctx promotes its JSON from an instantiation.
type Base[T any] struct{}

func (b *Base[T]) JSON(v T) {}

type Ctx struct{ Base[int] }

// Mux embeds an interface in a struct.
type Mux struct{ lib.Routes }

func run(app *App, e *lib.Engine, s Shadow, ctx Ctx, m Mux) {
	app.GET("/a")
	app.Use()
	e.GET("/e")
	e.Use()
	s.GET("/s")
	ctx.JSON(1)
	m.Mount("/m")
}
`

// promotedMeta type-checks the two packages the way the loader does — with
// Selections, which is the fact the promotion is read from — and builds their
// metadata.
func promotedMeta(t *testing.T) *Metadata {
	t.Helper()
	fset := token.NewFileSet()
	newInfo := func() *types.Info {
		return &types.Info{
			Types:      map[ast.Expr]types.TypeAndValue{},
			Defs:       map[*ast.Ident]types.Object{},
			Uses:       map[*ast.Ident]types.Object{},
			Instances:  map[*ast.Ident]types.Instance{},
			Selections: map[*ast.SelectorExpr]*types.Selection{},
		}
	}
	libFile, err := parser.ParseFile(fset, "lib.go", promotedLib, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse lib: %v", err)
	}
	libInfo := newInfo()
	libPkg, err := (&types.Config{}).Check("example.com/lib", fset, []*ast.File{libFile}, libInfo)
	if err != nil {
		t.Fatalf("typecheck lib: %v", err)
	}
	appFile, err := parser.ParseFile(fset, "app.go", promotedApp, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse app: %v", err)
	}
	appInfo := newInfo()
	conf := types.Config{Importer: importerFunc(func(path string) (*types.Package, error) {
		return libPkg, nil
	})}
	if _, err := conf.Check("example.com/app", fset, []*ast.File{appFile}, appInfo); err != nil {
		t.Fatalf("typecheck app: %v", err)
	}
	return GenerateMetadata(
		map[string]map[string]*ast.File{
			"example.com/lib": {"lib.go": libFile},
			"example.com/app": {"app.go": appFile},
		},
		map[*ast.File]*types.Info{libFile: libInfo, appFile: appInfo},
		map[string]string{"lib.go": "example.com/lib", "app.go": "example.com/app"},
		fset,
	)
}

type importerFunc func(path string) (*types.Package, error)

func (f importerFunc) Import(path string) (*types.Package, error) { return f(path) }

// TestPromotedMethodRecordsDeclaringReceiver locks in issue #643: a method
// reached through an embedded field is recorded on the type that DECLARES it,
// with the type the call was written against kept as a second fact. Recording
// only the written type named a method no one declares, so framework receiver
// patterns never matched `app.Group(...)` on `struct{ *gin.Engine }`.
func TestPromotedMethodRecordsDeclaringReceiver(t *testing.T) {
	meta := promotedMeta(t)
	sp := meta.StringPool

	type callee struct{ pkg, recv, written string }
	got := map[string]callee{}
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		if sp.GetString(e.Caller.Name) != "run" {
			continue
		}
		written := ""
		if w := e.Callee.WrittenRecvType(); w >= 0 {
			written = sp.GetString(w)
		}
		got[sp.GetString(e.Callee.Name)+"@"+sp.GetString(e.Callee.Position)] = callee{
			pkg: sp.GetString(e.Callee.Pkg), recv: sp.GetString(e.Callee.RecvType), written: written,
		}
	}

	want := map[string]struct {
		callee
		why string
	}{
		"GET@app.go:24:2":   {callee{"example.com/lib", "*Group", "example.com/app.*App"}, "promoted twice, from another package"},
		"Use@app.go:25:2":   {callee{"example.com/lib", "*Engine", "example.com/app.*App"}, "promoted once"},
		"GET@app.go:26:2":   {callee{"example.com/lib", "*Group", "example.com/lib.*Engine"}, "a framework's own embedding is promotion too"},
		"Use@app.go:27:2":   {callee{"example.com/lib", "*Engine", ""}, "declared on the receiver's own type: unchanged"},
		"GET@app.go:28:2":   {callee{"example.com/app", "Shadow", ""}, "a shadowing method keeps its own receiver"},
		"JSON@app.go:29:2":  {callee{"example.com/app", "*Base", "example.com/app.Ctx"}, "a generic origin is named by its declaration"},
		"Mount@app.go:30:2": {callee{"example.com/lib", "Routes", "example.com/app.Mux"}, "an interface embedded in a struct declares its method"},
	}
	for key, w := range want {
		g, ok := got[key]
		if !ok {
			t.Errorf("%s: no edge recorded; have %v", key, got)
			continue
		}
		if g != w.callee {
			t.Errorf("%s (%s): got %+v, want %+v", key, w.why, g, w.callee)
		}
	}
}
