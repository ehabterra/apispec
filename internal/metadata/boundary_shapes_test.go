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
	"testing"

	metadata "github.com/ehabterra/apispec/internal/metadata"
	"golang.org/x/tools/go/packages"
)

// TestMetadata_BoundaryShapeImprovements locks in the two type shapes the
// AST boundary lost before it rendered through the structured type model
// (phase 3 of docs/TYPE_MODEL.md):
//
//   - a fixed-size array field recorded "[]byte" — the length was dropped, so
//     the mapper could never emit maxItems/maxLength for it;
//   - a method on a multi-parameter generic type vanished entirely — the
//     receiver Pair[K, V] is an IndexListExpr, which stringified to "", so
//     the method was filed under the empty key and never attached.
func TestMetadata_BoundaryShapeImprovements(t *testing.T) {
	fset := token.NewFileSet()

	src := []testModule{{
		Name: "shapes",
		Files: map[string]interface{}{"main.go": `package main

type Pair[K any, V any] struct {
	First  K
	Second V
}

func (p Pair[K, V]) Sum() string { return "" }

type Packet struct {
	Header [4]byte
	Labels [2]string
}

func main() {}
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

	str := func(id int) string { return meta.StringPool.GetString(id) }
	findType := func(name string) *metadata.Type {
		for _, pkg := range meta.Packages {
			for _, file := range pkg.Files {
				if typ, ok := file.Types[name]; ok {
					return typ
				}
			}
		}
		return nil
	}

	// Fixed-size array fields keep their length.
	packet := findType("Packet")
	if packet == nil {
		t.Fatal("Packet type not found")
	}
	fieldTypes := map[string]string{}
	for _, f := range packet.Fields {
		fieldTypes[str(f.Name)] = str(f.Type)
	}
	if fieldTypes["Header"] != "[4]byte" {
		t.Errorf("Packet.Header type = %q, want \"[4]byte\" (length preserved)", fieldTypes["Header"])
	}
	if fieldTypes["Labels"] != "[2]string" {
		t.Errorf("Packet.Labels type = %q, want \"[2]string\" (length preserved)", fieldTypes["Labels"])
	}

	// Methods on a multi-parameter generic type attach to it.
	pair := findType("Pair")
	if pair == nil {
		t.Fatal("Pair type not found")
	}
	var methods []string
	for _, m := range pair.Methods {
		methods = append(methods, str(m.Name))
	}
	found := false
	for _, m := range methods {
		if m == "Sum" {
			found = true
		}
	}
	if !found {
		t.Errorf("Pair methods = %v, want Sum attached (IndexListExpr receiver)", methods)
	}
}
