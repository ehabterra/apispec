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
