package callgraph

import (
	"path/filepath"
	"reflect"
	"testing"

	"golang.org/x/tools/go/packages"
)

// loadFixture loads a testdata module with the same mode the engine uses
// (dependencies from export data, no dep bodies).
func loadFixture(t *testing.T, dir string) []*packages.Package {
	t.Helper()
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedTypesInfo | packages.NeedImports,
		Dir: abs,
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("packages.Load: %v", err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages contain errors")
	}
	return pkgs
}

const muxFixture = "../../testdata/mux_path_params"
const muxPkg = "testdata/mux_path_params"

func TestBuild_MuxReachability(t *testing.T) {
	r := Build(loadFixture(t, muxFixture))

	// Same facts the extractor's bounded meta.Callers walk computes today —
	// including the helper hop (getOrder -> pathVar -> mux.Vars) — but with
	// no depth limit.
	cases := []struct {
		from string
		want bool
	}{
		{muxPkg + ".getProduct", true},
		{muxPkg + ".getOrder", true},
		{muxPkg + ".getTag", true},
		{muxPkg + ".getItem", false},
	}
	for _, tc := range cases {
		if got := r.ReachesID(tc.from, "github.com/gorilla/mux.Vars"); got != tc.want {
			t.Errorf("ReachesID(%s, mux.Vars) = %v, want %v", tc.from, got, tc.want)
		}
	}
}

func TestBuild_FunctionIDFormats(t *testing.T) {
	r := Build(loadFixture(t, muxFixture))

	// Named module functions index under metadata-BaseID-style IDs.
	for _, id := range []string{
		muxPkg + ".main", muxPkg + ".pathVar", muxPkg + ".getOrder",
	} {
		if len(r.FunctionsByID(id)) == 0 {
			t.Errorf("FunctionsByID(%q) empty", id)
		}
	}
	// Dependency functions resolve to declared stubs and index too.
	if len(r.FunctionsByID("github.com/gorilla/mux.Vars")) == 0 {
		t.Error("dependency function mux.Vars not indexed")
	}
	// Methods use pkg.recv.name with the pointer star stripped.
	if len(r.FunctionsByID("github.com/gorilla/mux.Router.HandleFunc")) == 0 {
		t.Error("method ID pkg.recv.name (star stripped) not found for (*mux.Router).HandleFunc")
	}
}

func TestBuild_Deterministic(t *testing.T) {
	dump := func(r *Resolved) map[string][]string {
		out := make(map[string][]string)
		for _, id := range r.IDs() {
			for _, fn := range r.FunctionsByID(id) {
				var callees []string
				for _, c := range r.CalleesOf(fn) {
					callees = append(callees, FunctionID(c))
				}
				out[id] = append(out[id], callees...)
			}
		}
		return out
	}

	a := dump(Build(loadFixture(t, muxFixture)))
	b := dump(Build(loadFixture(t, muxFixture)))
	if !reflect.DeepEqual(a, b) {
		t.Error("two builds over the same packages produced different edge dumps")
	}
}

func TestFunctionID_Nil(t *testing.T) {
	if got := FunctionID(nil); got != "" {
		t.Errorf("FunctionID(nil) = %q, want empty", got)
	}
}
