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
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

const dispatchPkg = "example.com/cli"

// buildDispatchMeta assembles metadata for a source snippet: main's composite
// literal as an assignment value, a Command type declaration (each subtest sets
// its own fields), and a caller entry per name in `callable` so
// knownFunctionKeys accepts it.
//
// Written against the real ExprToCallArgument rather than hand-built arguments,
// because the shape of a *nested* literal is exactly what this index depends on
// and hand-building it would let the test agree with a wrong assumption.
func buildDispatchMeta(t *testing.T, src string, callable ...string) *metadata.Metadata {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "cli.go", src, 0)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	// The first composite literal in main becomes the assignment's value.
	var lit *metadata.CallArgument
	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "main" {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok && lit == nil {
				lit = metadata.ExprToCallArgument(cl, nil, dispatchPkg, fset, meta)
				return false
			}
			return true
		})
	}
	if lit == nil {
		t.Fatal("no composite literal found in main")
	}

	cmdType := &metadata.Type{
		Name: meta.StringPool.Get("Command"),
		Pkg:  meta.StringPool.Get(dispatchPkg),
		Kind: meta.StringPool.Get("struct"),
	}
	mainFn := &metadata.Function{
		Name: meta.StringPool.Get("main"),
		Pkg:  meta.StringPool.Get(dispatchPkg),
		AssignmentMap: map[string][]metadata.Assignment{
			"app": {{
				VariableName: meta.StringPool.Get("app"),
				Pkg:          meta.StringPool.Get(dispatchPkg),
				Func:         meta.StringPool.Get("main"),
				Value:        *lit,
			}},
		},
	}
	meta.Packages = map[string]*metadata.Package{
		dispatchPkg: {Files: map[string]*metadata.File{
			"cli.go": {
				Types:     map[string]*metadata.Type{"Command": cmdType},
				Functions: map[string]*metadata.Function{"main": mainFn},
			},
		}},
	}
	// knownFunctionKeys reads meta.Callers: only a key that can actually be
	// expanded belongs in the index.
	meta.Callers = map[string][]*metadata.CallGraphEdge{}
	for _, name := range callable {
		meta.Callers[dispatchPkg+"."+name] = nil
	}
	return meta
}

func field(meta *metadata.Metadata, name, typ string) metadata.Field {
	return metadata.Field{Name: meta.StringPool.Get(name), Type: meta.StringPool.Get(typ)}
}

// TestBuildFuncFieldDispatch covers what the index will and will not claim about
// a func-typed struct field (issue #143).
func TestBuildFuncFieldDispatch(t *testing.T) {
	t.Run("nested literal records the function value", func(t *testing.T) {
		src := `package cli
func runWeb() error { return nil }
func main() { app := &App{Commands: []*Command{{Name: "web", Action: runWeb}}}; _ = app }
`
		meta := buildDispatchMeta(t, src, "runWeb")
		meta.Packages[dispatchPkg].Files["cli.go"].Types["Command"].Fields = []metadata.Field{
			field(meta, "Name", "string"), field(meta, "Action", "func"),
		}

		got := buildFuncFieldDispatch(meta)
		want := []string{dispatchPkg + ".runWeb"}
		if diff := got[funcFieldKey(dispatchPkg, "Command", "Action")]; !reflect.DeepEqual(diff, want) {
			t.Errorf("Command.Action = %v, want %v (whole index: %v)", diff, want, got)
		}
	})

	t.Run("a field whose value is a variable resolves to nothing", func(t *testing.T) {
		// The variable names no body, so there is nothing honest to record —
		// chasing what it holds is value resolution, not this index's job.
		src := `package cli
func main() { var h func() error; app := &App{Commands: []*Command{{Name: "web", Action: h}}}; _ = app }
`
		meta := buildDispatchMeta(t, src, "runWeb")
		meta.Packages[dispatchPkg].Files["cli.go"].Types["Command"].Fields = []metadata.Field{
			field(meta, "Name", "string"), field(meta, "Action", "func"),
		}
		if got := buildFuncFieldDispatch(meta); len(got) != 0 {
			t.Errorf("expected no entries for a variable-valued field, got %v", got)
		}
	})

	t.Run("a non-func field is never indexed", func(t *testing.T) {
		// `Name: runWeb` cannot happen in compiling Go, but a field-name match on
		// a differently-typed field must not be enough on its own: the type is
		// what says "this is a dispatch point".
		src := `package cli
func runWeb() error { return nil }
func main() { app := &App{Commands: []*Command{{Name: "web", Action: runWeb}}}; _ = app }
`
		meta := buildDispatchMeta(t, src, "runWeb")
		meta.Packages[dispatchPkg].Files["cli.go"].Types["Command"].Fields = []metadata.Field{
			field(meta, "Name", "string"), field(meta, "Action", "string"), // NOT a func
		}
		if got := buildFuncFieldDispatch(meta); len(got) != 0 {
			t.Errorf("expected no entries when the field is not func-typed, got %v", got)
		}
	})

	t.Run("an unexpandable function is not recorded", func(t *testing.T) {
		// knownFunctionKeys is the expandable set; a name that calls nothing
		// contributes no routes and must not enter the index.
		src := `package cli
func runWeb() error { return nil }
func main() { app := &App{Commands: []*Command{{Name: "web", Action: runWeb}}}; _ = app }
`
		meta := buildDispatchMeta(t, src) // no callable functions at all
		meta.Packages[dispatchPkg].Files["cli.go"].Types["Command"].Fields = []metadata.Field{
			field(meta, "Name", "string"), field(meta, "Action", "func"),
		}
		if got := buildFuncFieldDispatch(meta); len(got) != 0 {
			t.Errorf("expected no entries for a function with no recorded calls, got %v", got)
		}
	})

	t.Run("several values for one field are all kept, sorted", func(t *testing.T) {
		// Each command is genuinely invoked for its own subcommand, so the union
		// is the binary's surface — and the order must not depend on map
		// iteration (golden rule #1).
		src := `package cli
func runWeb() error { return nil }
func runAdmin() error { return nil }
func main() {
	app := &App{Commands: []*Command{{Name: "web", Action: runWeb}, {Name: "admin", Action: runAdmin}}}
	_ = app
}
`
		meta := buildDispatchMeta(t, src, "runWeb", "runAdmin")
		meta.Packages[dispatchPkg].Files["cli.go"].Types["Command"].Fields = []metadata.Field{
			field(meta, "Name", "string"), field(meta, "Action", "func"),
		}
		got := buildFuncFieldDispatch(meta)[funcFieldKey(dispatchPkg, "Command", "Action")]
		want := []string{dispatchPkg + ".runAdmin", dispatchPkg + ".runWeb"}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("Command.Action = %v, want %v (sorted)", got, want)
		}
	})

	t.Run("positional literal binds elements to fields in order", func(t *testing.T) {
		src := `package cli
func runWeb() error { return nil }
func main() { app := &App{Commands: []*Command{{"web", runWeb}}}; _ = app }
`
		meta := buildDispatchMeta(t, src, "runWeb")
		meta.Packages[dispatchPkg].Files["cli.go"].Types["Command"].Fields = []metadata.Field{
			field(meta, "Name", "string"), field(meta, "Action", "func"),
		}
		got := buildFuncFieldDispatch(meta)[funcFieldKey(dispatchPkg, "Command", "Action")]
		if want := []string{dispatchPkg + ".runWeb"}; !reflect.DeepEqual(got, want) {
			t.Errorf("Command.Action = %v, want %v", got, want)
		}
	})

	t.Run("keysFor only answers for a recorded field", func(t *testing.T) {
		d := funcFieldDispatch{funcFieldKey(dispatchPkg, "Command", "Action"): {dispatchPkg + ".runWeb"}}
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		edge := func(pkg, recv, name string) *metadata.CallGraphEdge {
			return &metadata.CallGraphEdge{Callee: metadata.Call{
				Pkg:      meta.StringPool.Get(pkg),
				RecvType: meta.StringPool.Get(recv),
				Name:     meta.StringPool.Get(name),
			}}
		}
		if got := d.keysFor(meta, edge(dispatchPkg, "*Command", "Action")); len(got) != 1 {
			t.Errorf("pointer receiver should match the bare type name, got %v", got)
		}
		if got := d.keysFor(meta, edge(dispatchPkg, dispatchPkg+".Command", "Action")); len(got) != 1 {
			t.Errorf("package-qualified receiver should match, got %v", got)
		}
		for _, tc := range []struct{ pkg, recv, name string }{
			{dispatchPkg, "*Command", "Run"},      // different member
			{dispatchPkg, "*App", "Action"},       // different type
			{"other.com/x", "*Command", "Action"}, // different package
			{dispatchPkg, "", "Action"},           // no receiver: a plain function call
		} {
			if got := d.keysFor(meta, edge(tc.pkg, tc.recv, tc.name)); len(got) != 0 {
				t.Errorf("keysFor(%q, %q, %q) = %v, want none", tc.pkg, tc.recv, tc.name, got)
			}
		}
		if got := d.keysFor(meta, nil); got != nil {
			t.Errorf("keysFor(nil edge) = %v, want nil", got)
		}
		if got := funcFieldDispatch(nil).keysFor(meta, edge(dispatchPkg, "*Command", "Action")); got != nil {
			t.Errorf("empty index must answer nothing, got %v", got)
		}
	})

	t.Run("nil metadata is not a panic", func(t *testing.T) {
		if got := buildFuncFieldDispatch(nil); got != nil {
			t.Errorf("buildFuncFieldDispatch(nil) = %v, want nil", got)
		}
	})
}

func TestElementTypeName(t *testing.T) {
	for in, want := range map[string]string{
		"[]*Command":            "*Command",
		"map[string]*Command":   "*Command",
		"[4]*Command":           "*Command",
		"*Command":              "",
		"Command":               "",
		"":                      "",
		"map[string][]*Command": "[]*Command",
	} {
		if got := elementTypeName(in); got != want {
			t.Errorf("elementTypeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestBareTypeName(t *testing.T) {
	for in, want := range map[string]string{
		"*Command":                 "Command",
		"[]*Command":               "Command",
		"example.com/cli.Command":  "Command",
		"*example.com/cli.Command": "Command",
		"Command":                  "Command",
	} {
		if got := bareTypeName(in); got != want {
			t.Errorf("bareTypeName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsFuncTypeString(t *testing.T) {
	for in, want := range map[string]bool{
		"func":               true,
		"func(c *Ctx) error": true,
		"*func":              true,
		"string":             false,
		"":                   false,
		"funcy":              false,
	} {
		if got := isFuncTypeString(in); got != want {
			t.Errorf("isFuncTypeString(%q) = %v, want %v", in, got, want)
		}
	}
}

// TestFuncFieldDispatchGuards covers the paths a fixture cannot reach: malformed
// or partial metadata, and the value forms that must resolve to nothing rather
// than to a guess.
func TestFuncFieldDispatchGuards(t *testing.T) {
	t.Run("nil packages, files and functions are skipped", func(t *testing.T) {
		meta := &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages: map[string]*metadata.Package{
				"a": nil,
				"b": {Files: map[string]*metadata.File{"f.go": nil}},
				"c": {Files: map[string]*metadata.File{
					"f.go": {Functions: map[string]*metadata.Function{"main": nil}},
				}},
			},
		}
		if got := buildFuncFieldDispatch(meta); len(got) != 0 {
			t.Errorf("expected an empty index, got %v", got)
		}
	})

	t.Run("a nil call argument does not panic", func(t *testing.T) {
		meta := &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			CallGraph:  []metadata.CallGraphEdge{{Args: []*metadata.CallArgument{nil}}},
		}
		if got := buildFuncFieldDispatch(meta); len(got) != 0 {
			t.Errorf("expected an empty index, got %v", got)
		}
	})

	t.Run("functionKeysOfValue resolves only what names a function", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		known := map[string]bool{
			"example.com/web.RunWeb":         true,
			"example.com/web.Server.RunWeb":  true,
			"example.com/cli.FuncLit:f.go:9": true,
		}
		ident := func(name, pkg string) *metadata.CallArgument {
			a := metadata.NewCallArgument(meta)
			a.SetKind(metadata.KindIdent)
			a.SetName(name)
			if pkg != "" {
				a.SetPkg(pkg)
			}
			return a
		}

		// A cross-package function value: pkg.Fn, no receiver.
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		sel.Sel = ident("RunWeb", "example.com/web")
		sel.X = ident("web", "")
		if got := functionKeysOfValue(known, sel, "example.com/cli"); !reflect.DeepEqual(got, []string{"example.com/web.RunWeb"}) {
			t.Errorf("cross-package function value = %v", got)
		}

		// A method value: the receiver type completes the key.
		recv := ident("srv", "")
		recv.SetType("*example.com/web.Server")
		method := metadata.NewCallArgument(meta)
		method.SetKind(metadata.KindSelector)
		method.Sel = ident("RunWeb", "example.com/web")
		method.X = recv
		if got := functionKeysOfValue(known, method, "example.com/cli"); !reflect.DeepEqual(got, []string{"example.com/web.Server.RunWeb"}) {
			t.Errorf("method value = %v", got)
		}

		// A closure, named by position.
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindFuncLit)
		lit.SetName("FuncLit:f.go:9")
		if got := functionKeysOfValue(known, lit, "example.com/cli"); !reflect.DeepEqual(got, []string{"example.com/cli.FuncLit:f.go:9"}) {
			t.Errorf("closure value = %v", got)
		}

		// Nothing that fails to name an expandable function may resolve.
		empty := metadata.NewCallArgument(meta)
		empty.SetKind(metadata.KindSelector)
		bareSel := metadata.NewCallArgument(meta)
		bareSel.SetKind(metadata.KindSelector)
		bareSel.Sel = ident("", "")
		unknownFn := metadata.NewCallArgument(meta)
		unknownFn.SetKind(metadata.KindSelector)
		unknownFn.Sel = ident("Missing", "example.com/web")
		unknownFn.X = ident("web", "")
		literal := metadata.NewCallArgument(meta)
		literal.SetKind(metadata.KindLiteral)
		for name, arg := range map[string]*metadata.CallArgument{
			"nil value":             nil,
			"selector with no Sel":  empty,
			"selector with no name": bareSel,
			"unknown function":      unknownFn,
			"a literal":             literal,
			"unknown ident":         ident("nope", "example.com/cli"),
			"ident with no pkg":     ident("RunWeb", ""),
		} {
			if got := functionKeysOfValue(known, arg, ""); got != nil {
				t.Errorf("%s resolved to %v, want nothing", name, got)
			}
		}
	})

	t.Run("rendered values resolve through the file's imports", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		file := &metadata.File{Imports: map[int]int{
			// Unaliased: the path is recorded as its own alias, so the qualifier
			// has to be matched against the path's tail.
			meta.StringPool.Get("example.com/web"): meta.StringPool.Get("example.com/web"),
			// Explicit alias.
			meta.StringPool.Get("w2"): meta.StringPool.Get("example.com/web2"),
			// Module major version: the qualifier is the element before "v5".
			meta.StringPool.Get("github.com/go-chi/chi/v5"): meta.StringPool.Get("github.com/go-chi/chi/v5"),
		}}
		known := map[string]bool{
			"example.com/web.RunWeb":             true,
			"example.com/web2.RunWeb":            true,
			"github.com/go-chi/chi/v5.NewRouter": true,
			"example.com/cli.local":              true,
		}
		for value, want := range map[string]string{
			"web.RunWeb":    "example.com/web.RunWeb",
			"w2.RunWeb":     "example.com/web2.RunWeb",
			"chi.NewRouter": "github.com/go-chi/chi/v5.NewRouter",
			"local":         "example.com/cli.local",
			"nope.RunWeb":   "",
			"web.Missing":   "",
			"unqualified":   "",
			"":              "",
		} {
			if got := functionKeyFromRendered(meta, known, "example.com/cli", file, value); got != want {
				t.Errorf("functionKeyFromRendered(%q) = %q, want %q", value, got, want)
			}
		}
		// No import table at all: only the context package can answer.
		if got := functionKeyFromRendered(meta, known, "example.com/cli", nil, "web.RunWeb"); got != "" {
			t.Errorf("with no file = %q, want empty", got)
		}
	})
}

func TestSplitQualifiedType(t *testing.T) {
	for in, want := range map[string][2]string{
		"[]*github.com/x/clipkg.Command": {"github.com/x/clipkg", "Command"},
		"github.com/x/clipkg.Command":    {"github.com/x/clipkg", "Command"},
		// An alias-qualified name is NOT a package path: resolving it needs the
		// import table, and treating "clipkg" as a path would key the index on a
		// package that does not exist.
		"clipkg.Command": {"", "Command"},
		"[]*Command":     {"", "Command"},
		"Command":        {"", "Command"},
		"":               {"", ""},
	} {
		pkg, name := splitQualifiedType(in)
		if pkg != want[0] || name != want[1] {
			t.Errorf("splitQualifiedType(%q) = (%q, %q), want (%q, %q)", in, pkg, name, want[0], want[1])
		}
	}
}

func TestImportTailMatches(t *testing.T) {
	for _, tc := range []struct {
		path, qualifier string
		want            bool
	}{
		{"example.com/web", "web", true},
		{"github.com/go-chi/chi/v5", "chi", true},
		{"github.com/go-chi/chi/v5", "v5", true}, // the literal element still matches
		{"example.com/web", "other", false},
		{"", "web", false},
		{"example.com/web", "", false},
		{"v5", "v5", true},
	} {
		if got := importTailMatches(tc.path, tc.qualifier); got != tc.want {
			t.Errorf("importTailMatches(%q, %q) = %v, want %v", tc.path, tc.qualifier, got, tc.want)
		}
	}
	for _, tc := range []struct {
		in   string
		want bool
	}{{"v5", true}, {"v", false}, {"v5x", false}, {"web", false}, {"", false}} {
		if got := isMajorVersionElement(tc.in); got != tc.want {
			t.Errorf("isMajorVersionElement(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestDedupeSortedStrings(t *testing.T) {
	for _, tc := range []struct{ in, want []string }{
		{[]string{"a", "a", "b"}, []string{"a", "b"}},
		{[]string{"a"}, []string{"a"}},
		{nil, nil},
	} {
		got := dedupeSortedStrings(append([]string(nil), tc.in...))
		if len(got) != len(tc.want) {
			t.Errorf("dedupeSortedStrings(%v) = %v, want %v", tc.in, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("dedupeSortedStrings(%v) = %v, want %v", tc.in, got, tc.want)
				break
			}
		}
	}
}

func TestCompositeLitTypeHelpers(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	if got := compositeLitTypeName(nil); got != "" {
		t.Errorf("compositeLitTypeName(nil) = %q", got)
	}
	bare := metadata.NewCallArgument(meta)
	bare.SetKind(metadata.KindCompositeLit)
	if got := compositeLitTypeName(bare); got != "" {
		t.Errorf("an elided literal states no type, got %q", got)
	}
	if got := typeExprName(nil); got != "" {
		t.Errorf("typeExprName(nil) = %q", got)
	}
	unsupported := metadata.NewCallArgument(meta)
	unsupported.SetKind(metadata.KindLiteral)
	if got := typeExprName(unsupported); got != "" {
		t.Errorf("typeExprName(literal) = %q", got)
	}

	// compositeLitPkg walks the type expression for an owner and falls back to
	// the context package when the literal cannot name one.
	if got := compositeLitPkg(nil, "ctx"); got != "ctx" {
		t.Errorf("compositeLitPkg(nil) = %q, want the context package", got)
	}
	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindSelector)
	selName := metadata.NewCallArgument(meta)
	selName.SetKind(metadata.KindIdent)
	selName.SetName("Command")
	selName.SetPkg("example.com/clipkg")
	sel.Sel = selName
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindCompositeLit)
	lit.X = sel
	if got := compositeLitPkg(lit, "ctx"); got != "example.com/clipkg" {
		t.Errorf("compositeLitPkg(qualified) = %q, want the selector's package", got)
	}
}

// TestFuncFieldDispatchPartialMetadata covers the remaining shapes a fixture
// cannot produce: metadata that is well-formed but incomplete, and the
// cross-package field type whose package is carried in the type string itself.
func TestFuncFieldDispatchPartialMetadata(t *testing.T) {
	t.Run("keysFor needs a package and a name", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		d := funcFieldDispatch{funcFieldKey("p", "Command", "Action"): {"p.runWeb"}}
		noPkg := &metadata.CallGraphEdge{Callee: metadata.Call{
			RecvType: meta.StringPool.Get("*Command"),
			Name:     meta.StringPool.Get("Action"),
		}}
		noName := &metadata.CallGraphEdge{Callee: metadata.Call{
			RecvType: meta.StringPool.Get("*Command"),
			Pkg:      meta.StringPool.Get("p"),
		}}
		if got := d.keysFor(meta, noPkg); got != nil {
			t.Errorf("an edge with no callee package resolved to %v", got)
		}
		if got := d.keysFor(meta, noName); got != nil {
			t.Errorf("an edge with no callee name resolved to %v", got)
		}
	})

	t.Run("an unnamed struct-instance field is skipped", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		meta.Callers = map[string][]*metadata.CallGraphEdge{"p.runWeb": nil}
		file := &metadata.File{
			Types: map[string]*metadata.Type{"Command": {
				Fields: []metadata.Field{{
					Name: meta.StringPool.Get("Action"),
					Type: meta.StringPool.Get("func"),
				}},
			}},
			StructInstances: []metadata.StructInstance{{
				Type: meta.StringPool.Get("Command"),
				Fields: map[int]int{
					meta.StringPool.Get(""):       meta.StringPool.Get("runWeb"),
					meta.StringPool.Get("Action"): meta.StringPool.Get("runWeb"),
				},
			}},
		}
		meta.Packages = map[string]*metadata.Package{"p": {Files: map[string]*metadata.File{"f.go": file}}}
		got := buildFuncFieldDispatch(meta)
		if want := []string{"p.runWeb"}; len(got) != 1 || !reflect.DeepEqual(got[funcFieldKey("p", "Command", "Action")], want) {
			t.Errorf("index = %v, want only the named field to resolve", got)
		}
	})

	t.Run("a field type carrying its own package wins over the enclosing one", func(t *testing.T) {
		// `type App struct{ Cmd otherpkg.Command }` written in package "p", with
		// the field initialised by an ELIDED literal — legal Go, and the only
		// form where the literal states no type at all, so the field's own
		// (qualified) type is the sole source of the package to look fields up
		// in. Using the writing package instead finds nothing.
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		meta.Callers = map[string][]*metadata.CallGraphEdge{"p.runWeb": nil}

		fset := token.NewFileSet()
		src := `package p
func runWeb() error { return nil }
func main() { app := App{Cmd: {Action: runWeb}}; _ = app }
`
		f, err := parser.ParseFile(fset, "p.go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		var lit *metadata.CallArgument
		ast.Inspect(f, func(n ast.Node) bool {
			if cl, ok := n.(*ast.CompositeLit); ok && lit == nil {
				lit = metadata.ExprToCallArgument(cl, nil, "p", fset, meta)
				return false
			}
			return true
		})

		appType := &metadata.Type{Fields: []metadata.Field{{
			Name: meta.StringPool.Get("Cmd"),
			// Qualified: the field's type lives in another package.
			Type: meta.StringPool.Get("other.example/pkg.Command"),
		}}}
		cmdType := &metadata.Type{Fields: []metadata.Field{{
			Name: meta.StringPool.Get("Action"),
			Type: meta.StringPool.Get("func"),
		}}}
		meta.Packages = map[string]*metadata.Package{
			"p": {Files: map[string]*metadata.File{"p.go": {
				Types: map[string]*metadata.Type{"App": appType},
				Functions: map[string]*metadata.Function{"main": {
					AssignmentMap: map[string][]metadata.Assignment{
						"app": {{Value: *lit}},
					},
				}},
			}}},
			"other.example/pkg": {Files: map[string]*metadata.File{"cmd.go": {
				Types: map[string]*metadata.Type{"Command": cmdType},
			}}},
		}

		got := buildFuncFieldDispatch(meta)
		want := []string{"p.runWeb"}
		if key := funcFieldKey("other.example/pkg", "Command", "Action"); !reflect.DeepEqual(got[key], want) {
			t.Errorf("%s = %v, want %v (whole index: %v)", key, got[key], want, got)
		}
	})

	t.Run("a nil element inside a literal is skipped", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindCompositeLit)
		lit.Args = []*metadata.CallArgument{nil}
		meta.CallGraph = []metadata.CallGraphEdge{{Args: []*metadata.CallArgument{lit}}}
		if got := buildFuncFieldDispatch(meta); len(got) != 0 {
			t.Errorf("index = %v, want empty", got)
		}
	})

	t.Run("structFieldTypes and structFieldOrder skip nil files", func(t *testing.T) {
		meta := &metadata.Metadata{
			StringPool: metadata.NewStringPool(),
			Packages: map[string]*metadata.Package{"p": {Files: map[string]*metadata.File{
				"a.go": nil,
				"b.go": {Types: map[string]*metadata.Type{"Command": {Fields: []metadata.Field{{
					Name: metadata.NewStringPool().Get("Action"),
				}}}}},
			}}},
		}
		// The nil file must be stepped over rather than panicking; the declared
		// type in the other file is what answers.
		if got := structFieldTypes(meta, "p", "Command"); len(got) != 1 {
			t.Errorf("structFieldTypes = %v, want the one declared field", got)
		}
		if got := structFieldOrder(meta, "p", "Command"); len(got) != 1 {
			t.Errorf("structFieldOrder = %v, want the one declared field", got)
		}
	})

	t.Run("a selector whose Sel has no package falls back to the context", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		known := map[string]bool{"example.com/cli.RunWeb": true}
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindSelector)
		selName := metadata.NewCallArgument(meta)
		selName.SetKind(metadata.KindIdent)
		selName.SetName("RunWeb")
		sel.Sel = selName
		if got := functionKeysOfValue(known, sel, "example.com/cli"); !reflect.DeepEqual(got, []string{"example.com/cli.RunWeb"}) {
			t.Errorf("selector with no Sel package = %v", got)
		}
	})

	t.Run("an already-qualified rendered value is trusted as written", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		known := map[string]bool{"example.com/web.Server.RunAdmin": true}
		if got := functionKeyFromRendered(meta, known, "example.com/cli", &metadata.File{},
			"example.com/web.Server.RunAdmin"); got != "example.com/web.Server.RunAdmin" {
			t.Errorf("fully-qualified rendered value = %q", got)
		}
	})
}
