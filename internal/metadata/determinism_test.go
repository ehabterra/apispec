package metadata_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"golang.org/x/tools/go/packages"
	"gopkg.in/yaml.v3"
)

// determinismFixture is a single module with several packages, cross-package
// interface implementations, generics, and local anonymous structs — the
// constructs whose processing order historically flipped between runs
// (string-pool interning, Implements/ImplementedBy append order, info.Defs
// iteration).
var determinismFixture = testModule{
	Name: "determinism",
	Files: map[string]interface{}{
		"main.go": `package main

import (
	"determinism/api"
	"determinism/store"
)

func main() {
	s := store.New()
	api.Serve(s)
}
`,
		"api/api.go": `package api

import "determinism/store"

type Handler struct{ Repo store.Repo }

type Response[T any] struct {
	Data T      ` + "`json:\"data\"`" + `
	Err  string ` + "`json:\"err,omitempty\"`" + `
}

func Serve(r store.Repo) {
	h := Handler{Repo: r}
	_ = h
	local := struct {
		ID   int    ` + "`json:\"id\"`" + `
		Name string ` + "`json:\"name\"`" + `
	}{}
	_ = local
	_ = wrap(store.User{})
}

func wrap(u store.User) Response[store.User] {
	return Response[store.User]{Data: u}
}
`,
		"api/extra.go": `package api

type Namer interface{ Name() string }

type Titled struct{ Title string }

func (t Titled) Name() string { return t.Title }
`,
		"store/store.go": `package store

type Repo interface {
	Get(id int) User
}

type User struct {
	ID   int    ` + "`json:\"id\"`" + `
	Name string ` + "`json:\"name\"`" + `
}

func (u User) Name2() string { return u.Name }

type memRepo struct{ users map[int]User }

func New() Repo { return &memRepo{users: map[int]User{}} }

func (m *memRepo) Get(id int) User { return m.users[id] }
`,
		"store/extra.go": `package store

type Labeled interface{ Label() string }

func (u User) Label() string { return u.Name }
`,
	},
}

// generateOnce loads the fixture packages and runs metadata generation,
// returning the YAML serialization. Each call re-loads packages so that map
// iteration order differs between calls exactly as it does between real runs.
func generateOnce(t *testing.T, cfg *packages.Config) []byte {
	t.Helper()

	fset := token.NewFileSet()
	loadCfg := *cfg
	loadCfg.Mode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax |
		packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports
	loadCfg.Fset = fset
	loadCfg.Tests = false

	pkgs, err := packages.Load(&loadCfg, "./...")
	if err != nil {
		t.Fatal(err)
	}

	pkgsMetadata := map[string]map[string]*ast.File{}
	importPaths := map[string]string{}
	fileToInfo := map[*ast.File]*types.Info{}
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
	out, err := yaml.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

// TestGenerateMetadataDeterministic asserts that repeated metadata generation
// over identical input produces byte-identical YAML: same string-pool order,
// same call-graph edge order, same Implements/ImplementedBy order.
func TestGenerateMetadataDeterministic(t *testing.T) {
	cfg := exportModules(t, []testModule{determinismFixture})

	base := generateOnce(t, cfg)
	for run := 1; run < 3; run++ {
		got := generateOnce(t, cfg)
		if string(got) == string(base) {
			continue
		}
		t.Fatalf("run %d produced different metadata YAML than run 0:\n%s", run, firstDiff(string(base), string(got)))
	}
}

// firstDiff returns a short report of the first differing line between a and b.
func firstDiff(a, b string) string {
	al, bl := strings.Split(a, "\n"), strings.Split(b, "\n")
	n := len(al)
	if len(bl) < n {
		n = len(bl)
	}
	for i := 0; i < n; i++ {
		if al[i] != bl[i] {
			return fmt.Sprintf("line %d:\n  run0: %s\n  runN: %s", i+1, al[i], bl[i])
		}
	}
	return fmt.Sprintf("length differs: %d vs %d lines", len(al), len(bl))
}
