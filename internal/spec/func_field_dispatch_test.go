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
