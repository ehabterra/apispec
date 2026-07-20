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

package metadata_test

import (
	"go/ast"
	"go/token"
	"go/types"
	"slices"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"golang.org/x/tools/go/packages"
)

// TestExternalInterfaceImplementations covers issue #178: a user type that
// implements a standard-library interface must record it in Implements.
//
// The in-package analysis can only pair types it has recorded, and an imported
// package's interface declarations never become Type entries, so stdlib
// interfaces were absent entirely — blocking every "does T implement I?"
// question, including expanding a handler passed as an http.Handler value (#204).
func TestExternalInterfaceImplementations(t *testing.T) {
	fset := token.NewFileSet()
	src := []testModule{{
		Name: "extiface",
		Files: map[string]interface{}{"main.go": `package main

import (
	"fmt"
	"net/http"
)

// Greeter is a LOCAL interface: the in-package analysis already covered it.
type Greeter interface{ Greet() string }

// H implements the local Greeter, the stdlib http.Handler, and fmt.Stringer.
type H struct{}

func (h *H) Greet() string  { return "hi" }
func (h *H) String() string { return "H" }
func (h *H) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

// Unreferenced implements io.Writer, but io.Writer is never named in this
// source and never appears in a called signature — see the scope assertion.
type Unreferenced struct{}

func (u *Unreferenced) Write(p []byte) (int, error) { return 0, nil }

// Plain implements nothing.
type Plain struct{ N int }

// Generic exists to prove that constraint interfaces (comparable, type sets)
// are NOT recorded — nearly everything satisfies them, which is noise.
type Generic[T comparable] struct{ V T }

func main() {
	h := &H{}
	// http.Handler is never named here: it appears only as Handle's parameter
	// type, which is the dominant real-world shape.
	http.NewServeMux().Handle("/x", h)
	// fmt.Stringer IS named here, so it is in scope for recording.
	var _ fmt.Stringer = h
	_ = Generic[int]{}
	_ = Plain{}
	_ = &Unreferenced{}
}
`}}}

	cfg := exportModules(t, src)
	cfg.Mode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports
	cfg.Fset = fset
	cfg.Tests = false

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatal(err)
	}
	pkgsMetadata := map[string]map[string]*ast.File{}
	fileToInfo := map[*ast.File]*types.Info{}
	importPaths := map[string]string{}
	for _, pkg := range pkgs {
		if pkg.PkgPath == "" {
			continue
		}
		pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)
		for i, f := range pkg.Syntax {
			if i < len(pkg.GoFiles) {
				pkgsMetadata[pkg.PkgPath][pkg.GoFiles[i]] = f
				fileToInfo[f] = pkg.TypesInfo
				importPaths[pkg.GoFiles[i]] = pkg.PkgPath
			}
		}
	}

	meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

	implementsOf := func(name string) []string {
		for _, pkg := range meta.Packages {
			for _, file := range pkg.Files {
				if typ, ok := file.Types[name]; ok {
					out := make([]string, 0, len(typ.Implements))
					for _, idx := range typ.Implements {
						out = append(out, meta.StringPool.GetString(idx))
					}
					slices.Sort(out)
					return out
				}
			}
		}
		t.Fatalf("type %q not recorded", name)
		return nil
	}

	h := implementsOf("H")
	for _, want := range []string{"net/http.Handler", "fmt.Stringer", "extiface.Greeter"} {
		if !slices.Contains(h, want) {
			t.Errorf("H.Implements missing %q (#178); got %v", want, h)
		}
	}

	// Scope, asserted rather than left implicit: only interfaces the analyzed
	// code actually references are candidates — named in source, or present in
	// the signature of something it calls. Unreferenced implements io.Writer,
	// but nothing here mentions io.Writer, so the fact is not recorded. This is
	// deliberate (scanning every interface in every imported package would be
	// costly and mostly noise) and it covers the motivating cases, where the
	// interface is always in the registration's signature: http.Handler (#204),
	// http.ResponseWriter (#170), io.Reader (#153).
	if got := implementsOf("Unreferenced"); slices.Contains(got, "io.Writer") {
		t.Errorf("scope changed: io.Writer is now recorded without being referenced; got %v", got)
	}

	if got := implementsOf("Plain"); len(got) != 0 {
		t.Errorf("Plain implements nothing, got %v", got)
	}

	// Constraint interfaces must not be recorded: `comparable` is satisfied by
	// nearly every type, so recording it is noise rather than a behavioral fact.
	for _, name := range []string{"H", "Plain", "Generic"} {
		if slices.Contains(implementsOf(name), "comparable") {
			t.Errorf("%s.Implements records the `comparable` constraint", name)
		}
	}
}

// TestExternalInterfaceImplementationsDeterministic guards the ordering
// invariant (golden rule #1): Implements feeds tree expansion and the spec, so
// its order must not vary between runs.
func TestExternalInterfaceImplementationsDeterministic(t *testing.T) {
	var first []string
	for i := 0; i < 3; i++ {
		fset := token.NewFileSet()
		src := []testModule{{
			Name: "detiface",
			Files: map[string]interface{}{"main.go": `package main

import (
	"fmt"
	"io"
	"net/http"
)

type Multi struct{}

func (m *Multi) Write(p []byte) (int, error)                  { return 0, nil }
func (m *Multi) String() string                               { return "" }
func (m *Multi) ServeHTTP(w http.ResponseWriter, r *http.Request) {}

func main() {
	m := &Multi{}
	var _ io.Writer = m
	var _ fmt.Stringer = m
	http.NewServeMux().Handle("/x", m)
}
`}}}
		cfg := exportModules(t, src)
		cfg.Mode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports
		cfg.Fset = fset
		cfg.Tests = false
		pkgs, err := packages.Load(cfg, "./...")
		if err != nil {
			t.Fatal(err)
		}
		pkgsMetadata := map[string]map[string]*ast.File{}
		fileToInfo := map[*ast.File]*types.Info{}
		importPaths := map[string]string{}
		for _, pkg := range pkgs {
			if pkg.PkgPath == "" {
				continue
			}
			pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)
			for j, f := range pkg.Syntax {
				if j < len(pkg.GoFiles) {
					pkgsMetadata[pkg.PkgPath][pkg.GoFiles[j]] = f
					fileToInfo[f] = pkg.TypesInfo
					importPaths[pkg.GoFiles[j]] = pkg.PkgPath
				}
			}
		}
		meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

		var got []string
		for _, pkg := range meta.Packages {
			for _, file := range pkg.Files {
				if typ, ok := file.Types["Multi"]; ok {
					for _, idx := range typ.Implements {
						got = append(got, meta.StringPool.GetString(idx))
					}
				}
			}
		}
		if len(got) == 0 {
			t.Fatal("Multi recorded no Implements")
		}
		if i == 0 {
			first = got
			continue
		}
		if !slices.Equal(first, got) {
			t.Fatalf("Implements order varies between runs:\n run 0: %v\n run %d: %v", first, i, got)
		}
	}
}
