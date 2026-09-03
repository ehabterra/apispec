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
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"

	"github.com/ehabterra/apispec/internal/metadata"
)

// An inline struct has no doc comment, and the metadata has to say so with
// NoString rather than by leaving the field unset: zero is pool index 0, a
// real string, so the omission published whichever string the run pooled
// first as the schema's description — on the body and on every property of
// it (#448). The junk was arbitrary and moved with pooling order, so
// the same source drifted between versions ("literal" -> "func_type").
func TestAnonStructTypeHasNoComment(t *testing.T) {
	src := `package main

import (
	"encoding/json"
	"net/http"
)

func main() {
	http.HandleFunc("POST /thing", func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Code string ` + "`json:\"code\"`" + `
			Name string ` + "`json:\"name\"`" + `
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
	})
	_ = http.ListenAndServe(":8080", nil)
}
`
	cfg := exportModules(t, []testModule{{Name: "anoncomment", Files: map[string]interface{}{"main.go": src}}})
	fset := token.NewFileSet()
	cfg.Mode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports
	cfg.Fset = fset
	cfg.Tests = false

	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatal(err)
	}

	pkgFiles := map[string]map[string]*ast.File{}
	fileToInfo := map[*ast.File]*types.Info{}
	importPaths := map[string]string{}
	for _, pkg := range pkgs {
		if pkg.PkgPath == "" {
			continue
		}
		pkgFiles[pkg.PkgPath] = map[string]*ast.File{}
		for i, f := range pkg.Syntax {
			if i < len(pkg.GoFiles) {
				pkgFiles[pkg.PkgPath][pkg.GoFiles[i]] = f
				fileToInfo[f] = pkg.TypesInfo
				importPaths[pkg.GoFiles[i]] = pkg.PkgPath
			}
		}
	}

	meta := metadata.GenerateMetadata(pkgFiles, fileToInfo, importPaths, fset)

	// Slot 0 is now reserved for "" (#449), so an unset index no longer reads
	// as prose. The explicit NoString below is still what this test pins: the
	// reservation is a backstop, and stating "no comment" at the site is what
	// keeps the metadata honest if the reservation ever moves.
	if first := meta.StringPool.GetString(0); first != "" {
		t.Errorf("slot 0 should be the reserved empty string, got %q", first)
	}

	var checked int
	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			for name, typ := range file.Types {
				if !strings.Contains(name, metadata.AnonStructTypePrefix) {
					continue
				}
				checked++
				if typ.Comments != metadata.NoString {
					t.Errorf("anon struct %s: Comments = %d, want NoString (%d); it reads as %q",
						name, typ.Comments, metadata.NoString, meta.StringPool.GetString(typ.Comments))
				}
				for _, f := range typ.Fields {
					if f.Comments != metadata.NoString {
						t.Errorf("anon struct %s field %q: Comments = %d, want NoString (%d); it reads as %q",
							name, meta.StringPool.GetString(f.Name), f.Comments, metadata.NoString,
							meta.StringPool.GetString(f.Comments))
					}
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no anonymous-struct type was recorded, so nothing was verified")
	}
}
