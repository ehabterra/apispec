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
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"sort"
	"strings"
	"testing"
)

// typeCheckHandler parses and type-checks a snippet (which must declare a
// handler named `h`) and returns its body, the types.Info, and the fileset —
// everything detectMethodDispatch needs.
func typeCheckHandler(t *testing.T, body string) (*ast.BlockStmt, *types.Info, *token.FileSet) {
	t.Helper()
	src := "package p\n\nimport \"net/http\"\n\nfunc h(w http.ResponseWriter, r *http.Request) {\n" + body + "\n}\n"
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "h.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "h" {
			return fn.Body, info, fset
		}
	}
	t.Fatal("handler h not found")
	return nil, nil, nil
}

// methodsOf flattens a branch list to a sorted, comma-joined method string for
// order-independent comparison.
func methodsOf(branches []MethodBranch) string {
	var all []string
	for _, b := range branches {
		all = append(all, strings.Join(b.Methods, "+"))
	}
	sort.Strings(all)
	return strings.Join(all, ",")
}

func TestDetectMethodDispatch(t *testing.T) {
	cases := []struct {
		name string
		body string
		want string // methodsOf() of the result; "" means no dispatch
	}{
		{
			name: "switch with GET and POST",
			body: `switch r.Method {
			case http.MethodGet:
				_ = w
			case http.MethodPost:
				_ = r
			default:
				_ = w
			}`,
			want: "GET,POST",
		},
		{
			name: "if/else-if chain GET and DELETE",
			body: `if r.Method == http.MethodGet {
				_ = w
			} else if r.Method == http.MethodDelete {
				_ = r
			}`,
			want: "DELETE,GET",
		},
		{
			name: "multi-method case GET,HEAD",
			body: `switch r.Method {
			case http.MethodGet, http.MethodHead:
				_ = w
			}`,
			want: "GET+HEAD",
		},
		{
			name: "string literal case",
			body: `switch r.Method {
			case "GET":
				_ = w
			case "put":
				_ = r
			}`,
			want: "GET,PUT",
		},
		{
			name: "reversed if comparison (const == r.Method)",
			body: `if http.MethodPatch == r.Method {
				_ = w
			}`,
			want: "PATCH",
		},
		{
			name: "no dispatch — plain handler",
			body: `_ = w
			_ = r`,
			want: "",
		},
		{
			name: "switch on something else is ignored",
			body: `switch r.URL.Path {
			case "/a":
				_ = w
			}`,
			want: "",
		},
		{
			name: "default-only switch names no method",
			body: `switch r.Method {
			default:
				_ = w
			}`,
			want: "",
		},
		{
			name: "case string that is not a known verb is skipped",
			body: `switch r.Method {
			case "BOGUS":
				_ = w
			case http.MethodGet:
				_ = r
			}`,
			want: "GET",
		},
		{
			name: "non-constant case expression is skipped",
			body: `x := "GET"
			switch r.Method {
			case x:
				_ = w
			}`,
			want: "",
		},
		{
			name: "!= comparison is not a dispatch",
			body: `if r.Method != http.MethodGet {
				_ = w
			}`,
			want: "",
		},
		{
			name: "non-method if is ignored",
			body: `if r.URL.Path == "/x" {
				_ = w
			}`,
			want: "",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body, info, fset := typeCheckHandler(t, tc.body)
			got := methodsOf(detectMethodDispatch(body, info, fset))
			if got != tc.want {
				t.Errorf("detectMethodDispatch methods = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDetectMethodDispatch_LineRanges(t *testing.T) {
	body, info, fset := typeCheckHandler(t, `switch r.Method {
	case http.MethodGet:
		_ = w
	case http.MethodPost:
		_ = r
	}`)
	branches := detectMethodDispatch(body, info, fset)
	if len(branches) != 2 {
		t.Fatalf("want 2 branches, got %d", len(branches))
	}
	for _, b := range branches {
		if b.StartLine <= 0 || b.EndLine < b.StartLine {
			t.Errorf("branch %v has invalid line range [%d,%d]", b.Methods, b.StartLine, b.EndLine)
		}
	}
	// The two case ranges must not overlap (each verb owns distinct lines).
	if branches[0].EndLine >= branches[1].StartLine {
		t.Errorf("case ranges overlap: %+v", branches)
	}
}

func TestDetectMethodDispatch_NilSafety(t *testing.T) {
	if got := detectMethodDispatch(nil, nil, nil); got != nil {
		t.Errorf("nil inputs should yield nil, got %v", got)
	}
}

// typeCheckFile parses and type-checks a whole file, returning what
// detectLitDispatch needs.
func typeCheckFile(t *testing.T, src string) (*ast.File, *types.Info, *token.FileSet) {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "reg.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	info := &types.Info{
		Types: map[ast.Expr]types.TypeAndValue{},
		Defs:  map[*ast.Ident]types.Object{},
		Uses:  map[*ast.Ident]types.Object{},
	}
	conf := types.Config{Importer: importer.Default()}
	if _, err := conf.Check("p", fset, []*ast.File{file}, info); err != nil {
		t.Fatalf("typecheck: %v", err)
	}
	return file, info, fset
}

// TestDetectLitDispatch records a closure's dispatch under the closure's own
// identity, and keeps two closures written in one function apart — the whole
// point of the record, since the enclosing declaration's MethodDispatch holds
// both sets of arms with nothing to tell them apart (issue #382).
func TestDetectLitDispatch(t *testing.T) {
	file, info, fset := typeCheckFile(t, `package p

import "net/http"

func register() {
	http.HandleFunc("/a", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = w
		case http.MethodPost:
			_ = r
		}
	})
	http.HandleFunc("/b", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			_ = w
		}
	})
	http.HandleFunc("/c", func(w http.ResponseWriter, r *http.Request) {
		_ = w // no dispatch: no entry
	})
}
`)
	meta := &Metadata{StringPool: NewStringPool()}
	detectLitDispatch(file, info, "p", fset, meta)

	if len(meta.LitDispatch) != 2 {
		t.Fatalf("want an entry for each DISPATCHING closure (2), got %d: %v",
			len(meta.LitDispatch), sortedLitKeys(meta.LitDispatch))
	}
	byMethods := map[string]DispatchScope{}
	for key, lit := range meta.LitDispatch {
		if !strings.HasPrefix(key, "p."+FuncLitPrefix) {
			t.Errorf("key %q should be the closure's identity (pkg.FuncLit:<position>)", key)
		}
		byMethods[methodsOf(lit.Branches)] = lit
	}
	for _, want := range []string{"GET,POST", "DELETE"} {
		lit, ok := byMethods[want]
		if !ok {
			t.Fatalf("no entry with arms %q; have %v", want, sortedLitKeys(meta.LitDispatch))
		}
		// The body range is what scopes the arms to THIS literal.
		if lit.Body.StartLine <= 0 || lit.Body.EndLine < lit.Body.StartLine || lit.Body.StartCol <= 0 {
			t.Errorf("arms %q: body range %+v is not usable", want, lit.Body)
		}
		for _, b := range lit.Branches {
			if b.StartLine < lit.Body.StartLine || b.EndLine > lit.Body.EndLine {
				t.Errorf("arms %q: branch %v at [%d,%d] falls outside the literal body %+v",
					want, b.Methods, b.StartLine, b.EndLine, lit.Body)
			}
		}
		if meta.StringPool.GetString(lit.File) != "reg.go" {
			t.Errorf("arms %q: file = %q, want the absolute path of the literal's file",
				want, meta.StringPool.GetString(lit.File))
		}
	}
}

// A closure inside a METHOD is recorded too: methods are not in File.Functions,
// so a per-declaration walk would miss the `func (s *Server) routes()` shape
// entirely.
func TestDetectLitDispatchInsideMethod(t *testing.T) {
	file, info, fset := typeCheckFile(t, `package p

import "net/http"

type server struct{}

func (s *server) routes() {
	http.HandleFunc("/x", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPut:
			_ = w
		}
	})
}
`)
	meta := &Metadata{StringPool: NewStringPool()}
	detectLitDispatch(file, info, "p", fset, meta)
	if len(meta.LitDispatch) != 1 {
		t.Fatalf("want the method's closure recorded, got %d entries", len(meta.LitDispatch))
	}
	for _, lit := range meta.LitDispatch {
		if got := methodsOf(lit.Branches); got != "PUT" {
			t.Errorf("arms = %q, want PUT", got)
		}
	}
}

func sortedLitKeys(m map[string]DispatchScope) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// TestDetectBodyDispatch records a method body's arms together with the range
// they have to be scoped against — Method carries a Position but no EndLine, so
// without the range its arms could only be compared against the whole file
// (issue #427).
func TestDetectBodyDispatch(t *testing.T) {
	body, info, fset := typeCheckHandler(t, `switch r.Method {
	case http.MethodGet:
		_ = w
	case http.MethodPost:
		_ = r
	}`)
	meta := &Metadata{StringPool: NewStringPool()}

	ds := detectBodyDispatch(body, info, fset, meta)
	if ds == nil {
		t.Fatal("a body dispatching on r.Method must produce a scope")
	}
	if got := methodsOf(ds.Branches); got != "GET,POST" {
		t.Errorf("arms = %q, want GET,POST", got)
	}
	if meta.StringPool.GetString(ds.File) != "h.go" {
		t.Errorf("file = %q, want the file the body is written in", meta.StringPool.GetString(ds.File))
	}
	if ds.Body.StartLine <= 0 || ds.Body.EndLine < ds.Body.StartLine || ds.Body.StartCol <= 0 {
		t.Errorf("body range %+v is not usable", ds.Body)
	}
	for _, b := range ds.Branches {
		if b.StartLine < ds.Body.StartLine || b.EndLine > ds.Body.EndLine {
			t.Errorf("branch %v at [%d,%d] falls outside the body %+v",
				b.Methods, b.StartLine, b.EndLine, ds.Body)
		}
	}
}

// A body that does not dispatch records nothing, so the field stays nil on the
// overwhelming majority of methods.
func TestDetectBodyDispatchNoDispatch(t *testing.T) {
	body, info, fset := typeCheckHandler(t, `_ = w
	_ = r`)
	meta := &Metadata{StringPool: NewStringPool()}
	if ds := detectBodyDispatch(body, info, fset, meta); ds != nil {
		t.Errorf("a body with no r.Method dispatch must record nothing, got %+v", ds)
	}
	if ds := detectBodyDispatch(nil, info, fset, meta); ds != nil {
		t.Error("a bodyless declaration must record nothing")
	}
	if ds := detectBodyDispatch(body, info, fset, nil); ds != nil {
		t.Error("no metadata means no string pool to record into")
	}
}

// The dispatch of a method reaches its Method record end to end, which is what
// the spec layer resolves a method handler through.
func TestMethodDispatchRecordedOnMethod(t *testing.T) {
	file, info, fset := typeCheckFile(t, `package p

import "net/http"

type H struct{}

func (h *H) Users(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		_ = w
	case http.MethodPost:
		_ = r
	}
}

func (h *H) Plain(w http.ResponseWriter, r *http.Request) {
	_ = w
}
`)
	pkgs := map[string]map[string]*ast.File{"p": {"reg.go": file}}
	md := GenerateMetadata(pkgs, map[*ast.File]*types.Info{file: info}, map[string]string{"p": "p"}, fset)

	users := findTestMethod(t, md, "p", "Users")
	if users.Dispatch == nil {
		t.Fatal("the dispatching method must carry its dispatch")
	}
	if got := methodsOf(users.Dispatch.Branches); got != "GET,POST" {
		t.Errorf("arms = %q, want GET,POST", got)
	}
	if plain := findTestMethod(t, md, "p", "Plain"); plain.Dispatch != nil {
		t.Errorf("a method that does not dispatch must carry nothing, got %+v", plain.Dispatch)
	}
}

// findTestMethod returns the named method from the package's per-Type tables.
func findTestMethod(t *testing.T, md *Metadata, pkg, name string) *Method {
	t.Helper()
	for _, p := range md.Packages {
		for _, typ := range p.Types {
			for i := range typ.Methods {
				if md.StringPool.GetString(typ.Methods[i].Name) == name {
					return &typ.Methods[i]
				}
			}
		}
	}
	t.Fatalf("method %s.%s not found", pkg, name)
	return nil
}
