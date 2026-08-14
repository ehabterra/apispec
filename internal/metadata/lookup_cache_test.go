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
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// cacheTestMeta builds metadata with three edges, two of which share a callee
// (name, pkg) — so "the first match" is a meaningful claim.
func cacheTestMeta() *Metadata {
	m := &Metadata{StringPool: NewStringPool()}
	edge := func(caller, callee, calleePkg string) CallGraphEdge {
		return CallGraphEdge{
			Caller: Call{Name: m.StringPool.Get(caller), Pkg: m.StringPool.Get("app"), Meta: m},
			Callee: Call{Name: m.StringPool.Get(callee), Pkg: m.StringPool.Get(calleePkg), Meta: m},
		}
	}
	m.CallGraph = []CallGraphEdge{
		edge("first", "target", "app"),  // the first edge whose callee is app.target
		edge("other", "elsewhere", "x"), // noise between the two matches
		edge("second", "target", "app"), // a later edge with the same callee
	}
	return m
}

// TestCalleeEdgesByNamePkg pins the contract that lets the index replace a linear
// scan of the whole call graph: same edges, same order, and no answer at all
// while the graph is still being built.
func TestCalleeEdgesByNamePkg(t *testing.T) {
	m := cacheTestMeta()
	name := m.StringPool.Get("target")
	pkg := m.StringPool.Get("app")

	t.Run("no answer before the index is built", func(t *testing.T) {
		// TraceVariableOrigin runs during call-graph construction, when no index
		// exists; those callers must fall back to scanning rather than see an
		// empty result.
		if _, ok := m.calleeEdgesByNamePkg(name, pkg); ok {
			t.Error("index answered before BuildCallGraphMaps")
		}
	})

	m.BuildCallGraphMaps()

	t.Run("matching edges in CallGraph order", func(t *testing.T) {
		edges, ok := m.calleeEdgesByNamePkg(name, pkg)
		if !ok {
			t.Fatal("index did not answer after BuildCallGraphMaps")
		}
		var callers []string
		for _, e := range edges {
			callers = append(callers, m.StringPool.GetString(e.Caller.Name))
		}
		// Order is the whole point: the caller takes the FIRST match, so the
		// bucket must list them exactly as a scan of CallGraph would.
		if want := []string{"first", "second"}; !reflect.DeepEqual(callers, want) {
			t.Errorf("callers = %v, want %v", callers, want)
		}
	})

	t.Run("equivalent to scanning the graph", func(t *testing.T) {
		// The property the optimisation rests on, checked directly rather than
		// argued: for every callee in the graph, the index yields what a scan
		// yields.
		for i := range m.CallGraph {
			c := &m.CallGraph[i].Callee
			var scanned []*CallGraphEdge
			for j := range m.CallGraph {
				e := &m.CallGraph[j]
				if e.Callee.Name == c.Name && e.Callee.Pkg == c.Pkg {
					scanned = append(scanned, e)
				}
			}
			indexed, ok := m.calleeEdgesByNamePkg(c.Name, c.Pkg)
			if !ok {
				t.Fatalf("index stopped answering for edge %d", i)
			}
			if !reflect.DeepEqual(indexed, scanned) {
				t.Errorf("edge %d: index and scan disagree", i)
			}
		}
	})

	t.Run("a grown graph invalidates the index", func(t *testing.T) {
		m.CallGraph = append(m.CallGraph, CallGraphEdge{
			Caller: Call{Name: m.StringPool.Get("late"), Pkg: m.StringPool.Get("app"), Meta: m},
			Callee: Call{Name: name, Pkg: pkg, Meta: m},
		})
		if _, ok := m.calleeEdgesByNamePkg(name, pkg); ok {
			t.Error("index answered for a graph it was not built for — it would miss the new edge")
		}
		m.BuildCallGraphMaps()
		edges, ok := m.calleeEdgesByNamePkg(name, pkg)
		if !ok || len(edges) != 3 {
			t.Errorf("after rebuild: ok=%v edges=%d, want ok and 3", ok, len(edges))
		}
	})

	t.Run("an unknown callee has no edges", func(t *testing.T) {
		edges, ok := m.calleeEdgesByNamePkg(m.StringPool.Get("nope"), pkg)
		if !ok || len(edges) != 0 {
			t.Errorf("unknown callee: ok=%v edges=%v", ok, edges)
		}
	})
}

// TestTypeStringOf covers the memoized type renderer, including the paths a
// caller can hit before metadata exists.
func TestTypeStringOf(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	typ := types.Typ[types.String]

	if got := typeStringOf(m, nil); got != "" {
		t.Errorf("nil type = %q, want empty", got)
	}
	if got := typeStringOf(nil, typ); got != typ.String() {
		t.Errorf("nil metadata = %q, want %q", got, typ.String())
	}

	first := typeStringOf(m, typ)
	if first != typ.String() {
		t.Errorf("rendered %q, want %q", first, typ.String())
	}
	// Second call must come from the cache and be identical — the whole premise
	// is that rendering is a pure function of the type.
	if second := typeStringOf(m, typ); second != first {
		t.Errorf("cached call returned %q, want %q", second, first)
	}
	if len(m.typeStrCache) != 1 {
		t.Errorf("cache holds %d entries, want 1", len(m.typeStrCache))
	}

	// A different type gets its own entry rather than reusing one.
	if got := typeStringOf(m, types.Typ[types.Int]); got != "int" {
		t.Errorf("int rendered as %q", got)
	}
}

// TestImplementersOf covers the memoized implementers query: sorted order, and
// the guards.
func TestImplementersOf(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	iface := "net/http.Handler"
	want := m.StringPool.Get(iface)
	withImpl := &Type{Implements: []int{want}}
	without := &Type{}

	// Two packages, each with an implementer and a non-implementer; the names are
	// chosen so map order and sorted order differ.
	m.Packages = map[string]*Package{
		"z/pkg": {Types: map[string]*Type{"Zebra": withImpl, "Nope": without}},
		"a/pkg": {Types: map[string]*Type{"Apple": withImpl, "Other": without}},
	}

	got := m.ImplementersOf(iface)
	expected := []string{"a/pkg.Apple", "z/pkg.Zebra"}
	if !reflect.DeepEqual(got, expected) {
		t.Errorf("ImplementersOf = %v, want %v (sorted, implementers only)", got, expected)
	}
	// Cached call: same content, and the cache is what answers.
	if again := m.ImplementersOf(iface); !reflect.DeepEqual(again, expected) {
		t.Errorf("cached ImplementersOf = %v, want %v", again, expected)
	}
	if len(m.implementersCache) != 1 {
		t.Errorf("cache holds %d entries, want 1", len(m.implementersCache))
	}

	if got := m.ImplementersOf(""); got != nil {
		t.Errorf("empty key = %v, want nil", got)
	}
	if got := m.ImplementersOf("nobody.Implements.This"); got != nil {
		t.Errorf("unknown interface = %v, want nil", got)
	}
	var nilMeta *Metadata
	if got := nilMeta.ImplementersOf(iface); got != nil {
		t.Errorf("nil metadata = %v, want nil", got)
	}
}

// TestSortedFileNames covers the cached per-package file ordering — the order is
// load-bearing (lookups take the first file that declares a name), so this pins
// that caching cannot change it.
func TestSortedFileNames(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{"z.go": {}, "a.go": {}, "m.go": {}}},
	}

	want := []string{"a.go", "m.go", "z.go"}
	if got := m.SortedFileNames("app"); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedFileNames = %v, want %v", got, want)
	}
	if got := m.SortedFileNames("app"); !reflect.DeepEqual(got, want) {
		t.Errorf("cached SortedFileNames = %v, want %v", got, want)
	}

	// A package that gains a file must not keep serving the stale order — the
	// metadata is still being assembled while some lookups run.
	m.Packages["app"].Files["b.go"] = &File{}
	if got := m.SortedFileNames("app"); !reflect.DeepEqual(got, []string{"a.go", "b.go", "m.go", "z.go"}) {
		t.Errorf("after adding a file = %v, want the new name in sorted position", got)
	}

	if got := m.SortedFileNames("missing"); got != nil {
		t.Errorf("unknown package = %v, want nil", got)
	}
}

func TestSortedTypeNames(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{
			"a.go": {Types: map[string]*Type{"Zebra": {}, "Apple": {}, "Mango": {}}},
			"b.go": {Types: map[string]*Type{"Only": {}}},
		}},
	}

	want := []string{"Apple", "Mango", "Zebra"}
	if got := m.SortedTypeNames("app", "a.go"); !reflect.DeepEqual(got, want) {
		t.Errorf("SortedTypeNames = %v, want %v", got, want)
	}
	if got := m.SortedTypeNames("app", "a.go"); !reflect.DeepEqual(got, want) {
		t.Errorf("cached SortedTypeNames = %v, want %v", got, want)
	}
	// Per file, not per package: b.go must not see a.go's names.
	if got := m.SortedTypeNames("app", "b.go"); !reflect.DeepEqual(got, []string{"Only"}) {
		t.Errorf("b.go = %v, want [Only]", got)
	}

	// A file that gains a type must not keep serving the stale order — these
	// lookups run while metadata is still being assembled.
	m.Packages["app"].Files["a.go"].Types["Banana"] = &Type{}
	if got := m.SortedTypeNames("app", "a.go"); !reflect.DeepEqual(got, []string{"Apple", "Banana", "Mango", "Zebra"}) {
		t.Errorf("after adding a type = %v, want the new name in sorted position", got)
	}

	if got := m.SortedTypeNames("missing", "a.go"); got != nil {
		t.Errorf("unknown package = %v, want nil", got)
	}
	if got := m.SortedTypeNames("app", "missing.go"); got != nil {
		t.Errorf("unknown file = %v, want nil", got)
	}
}

// TestTypeInPackageMatchesSortedScan is the correctness guard for the index:
// it must answer exactly what scanning the files in sorted order answered,
// including which declaration wins when a name appears in two files. Go
// forbids that, but the replaced code had a defined answer and the index must
// not quietly pick the other one.
func TestTypeInPackageMatchesSortedScan(t *testing.T) {
	inA, inB, inC := &Type{Name: 1}, &Type{Name: 2}, &Type{Name: 3}
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{
			"z.go": {Types: map[string]*Type{"Dup": inC, "OnlyZ": {}}},
			"a.go": {Types: map[string]*Type{"Dup": inA, "OnlyA": {}}},
			"m.go": {Types: map[string]*Type{"Dup": inB}},
		}},
	}

	// Reference: the scan the index replaced.
	scan := func(pkgName, typeName string) *Type {
		pkg := m.Packages[pkgName]
		for _, fileName := range m.SortedFileNames(pkgName) {
			if typ, ok := pkg.Files[fileName].Types[typeName]; ok {
				return typ
			}
		}
		return nil
	}

	for _, name := range []string{"Dup", "OnlyA", "OnlyZ", "Absent"} {
		if got, want := m.TypeInPackage("app", name), scan("app", name); got != want {
			t.Errorf("TypeInPackage(%q) = %p, sorted scan = %p", name, got, want)
		}
	}
	// State it directly too: a.go sorts first, so its declaration wins.
	if got := m.TypeInPackage("app", "Dup"); got != inA {
		t.Errorf("Dup resolved to the declaration in the wrong file: %p, want a.go's %p", got, inA)
	}
}

// TestTypeInPackageInvalidatesOnGrowth covers the reason the index carries a
// (files, types) fingerprint rather than being built once: these lookups can
// run before metadata has finished being assembled, and a package that gains a
// type afterwards must not keep answering from the stale index.
func TestTypeInPackageInvalidatesOnGrowth(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{"a.go": {Types: map[string]*Type{"First": {}}}}},
	}
	if m.TypeInPackage("app", "First") == nil {
		t.Fatal("First should resolve")
	}

	// A type added to an existing file.
	added := &Type{Name: 7}
	m.Packages["app"].Files["a.go"].Types["Second"] = added
	if got := m.TypeInPackage("app", "Second"); got != added {
		t.Errorf("type added to an existing file = %p, want %p", got, added)
	}

	// A whole new file.
	inNew := &Type{Name: 8}
	m.Packages["app"].Files["b.go"] = &File{Types: map[string]*Type{"Third": inNew}}
	if got := m.TypeInPackage("app", "Third"); got != inNew {
		t.Errorf("type in a newly added file = %p, want %p", got, inNew)
	}

	if got := m.TypeInPackage("missing", "First"); got != nil {
		t.Errorf("unknown package = %p, want nil", got)
	}
}

// TestLookupCachesAreRaceFree exercises the two caches concurrently; the value
// of this test is under -race, which CI runs.
func TestLookupCachesAreRaceFree(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{
			"a.go": {Types: map[string]*Type{"A": {}, "B": {}}},
			"b.go": {Types: map[string]*Type{"C": {}}},
		}},
	}

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_ = m.TypeInPackage("app", "A")
				_ = m.TypeInPackage("app", "C")
				_ = m.SortedTypeNames("app", "a.go")
				_ = m.SortedFileNames("app")
			}
		}()
	}
	wg.Wait()
}

// TestLookupCachesTolerateNilFiles covers a nil entry in Package.Files. Every
// reader of that map in the spec layer skips nil, and the index build here
// does too — the shape fingerprint did not, so computing it panicked before a
// single lookup could run. Metadata is deserialised from YAML as well as
// generated, so a package need not be as well-formed as the generator makes it.
func TestLookupCachesTolerateNilFiles(t *testing.T) {
	present := &Type{Name: 1}
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{
			"a.go": {Types: map[string]*Type{"Present": present}},
			"b.go": nil,
		}},
	}

	if got := m.TypeInPackage("app", "Present"); got != present {
		t.Errorf("TypeInPackage = %p, want %p", got, present)
	}
	if got := m.TypeInPackage("app", "Absent"); got != nil {
		t.Errorf("absent type = %p, want nil", got)
	}
	if got := m.SortedTypeNames("app", "b.go"); got != nil {
		t.Errorf("SortedTypeNames of a nil file = %v, want nil", got)
	}
	if got := m.SortedTypeNames("app", "a.go"); !reflect.DeepEqual(got, []string{"Present"}) {
		t.Errorf("SortedTypeNames = %v, want [Present]", got)
	}
}

// TestFunctionInPackageMatchesFileScan pins the function index against the scan
// it replaced, including the case that decides determinism: a bare name that
// two files of one package declare must resolve to the same one every run.
func TestFunctionInPackageMatchesFileScan(t *testing.T) {
	inA, inZ := &Function{Name: 1}, &Function{Name: 2}
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{
			"z.go": {Functions: map[string]*Function{"Dup": inZ, "OnlyZ": {}}},
			"a.go": {Functions: map[string]*Function{"Dup": inA, "OnlyA": {}}},
		}},
	}

	for _, name := range []string{"OnlyA", "OnlyZ"} {
		if m.FunctionInPackage("app", name) == nil {
			t.Errorf("FunctionInPackage(%q) = nil, want the declaration", name)
		}
	}
	if got := m.FunctionInPackage("app", "Absent"); got != nil {
		t.Errorf("FunctionInPackage(\"Absent\") = %p, want nil", got)
	}
	// Go itself forbids the duplicate, so this is about the index never letting
	// map order decide an answer (golden rule #1): a.go sorts first, so a.go's
	// declaration wins, on this run and every other. Rebuilt repeatedly because
	// a map-order-dependent index can agree with the sorted one by chance.
	for range 32 {
		m.funcIndex, m.funcIndexFor = nil, nil // force a rebuild
		if got := m.FunctionInPackage("app", "Dup"); got != inA {
			t.Fatalf("Dup resolved to %p, want a.go's %p — the index must not depend on map order", got, inA)
		}
	}
	if got := m.FunctionInPackage("missing", "Dup"); got != nil {
		t.Errorf("FunctionInPackage on an unknown package = %p, want nil", got)
	}
}

// TestFunctionInPackageInvalidatesOnGrowth covers the reason the index carries a
// pkgShape fingerprint: these lookups can run before assembly has finished, and
// pkgShape counts functions (not only files) because assembly adds a function to
// a file that already exists.
func TestFunctionInPackageInvalidatesOnGrowth(t *testing.T) {
	m := &Metadata{StringPool: NewStringPool()}
	m.Packages = map[string]*Package{
		"app": {Files: map[string]*File{"a.go": {Functions: map[string]*Function{"First": {}}}}},
	}
	if m.FunctionInPackage("app", "First") == nil {
		t.Fatal("First should resolve")
	}
	// Added to the EXISTING file: the file count is unchanged, so only the
	// function count can notice this.
	m.Packages["app"].Files["a.go"].Functions["Second"] = &Function{}
	if m.FunctionInPackage("app", "Second") == nil {
		t.Error("a function added to an existing file must not be hidden by the stale index")
	}
	m.Packages["app"].Files["b.go"] = &File{Functions: map[string]*Function{"Third": {}}}
	if m.FunctionInPackage("app", "Third") == nil {
		t.Error("a function added in a new file must not be hidden by the stale index")
	}
}

// TestFunctionAnywhereIsDeterministic pins the fallback's contract: the first
// package in SORTED order that declares the bare name wins, so a name declared
// by several packages cannot resolve differently between runs (golden rule #1).
func TestFunctionAnywhereIsDeterministic(t *testing.T) {
	inApp, inZoo := &Function{Name: 1}, &Function{Name: 2}
	newMeta := func() *Metadata {
		m := &Metadata{StringPool: NewStringPool()}
		m.Packages = map[string]*Package{
			"zoo": {Files: map[string]*File{"z.go": {Functions: map[string]*Function{"Shared": inZoo}}}},
			"app": {Files: map[string]*File{"a.go": {Functions: map[string]*Function{"Shared": inApp, "Only": {}}}}},
		}
		return m
	}
	for range 32 {
		if got := newMeta().FunctionAnywhere("Shared"); got != inApp {
			t.Fatalf("Shared resolved to %p, want app's %p — \"app\" sorts before \"zoo\"", got, inApp)
		}
	}
	if got := newMeta().FunctionAnywhere("Absent"); got != nil {
		t.Errorf("FunctionAnywhere(\"Absent\") = %p, want nil", got)
	}
	if got := newMeta().FunctionAnywhere("Only"); got == nil {
		t.Error("a name declared by exactly one package must still resolve")
	}
}

// TestFunctionAnywhereIsSpecLayerOnly enforces the invariant that lets
// FunctionAnywhere memoize once instead of re-fingerprinting the program on
// every lookup: it may only be called from the spec layer. Spec-layer code does
// not run until GenerateMetadata has returned, so the declaration set it indexes
// is already final. A caller that runs DURING assembly would cache a snapshot
// that later lookups answer from — the bug pkgShape exists to prevent for
// FunctionInPackage, which needs its guard precisely because it lacks this
// property.
//
// Scanned repo-wide rather than package-locally: assembly is driven from this
// package today, but a future analysis pass called from GenerateMetadata could
// live anywhere, and it is the CALLER'S PHASE that matters. Allowing only
// internal/spec is the enforceable form of that.
//
// If this fails, either move the caller into the spec layer or give
// FunctionAnywhere a shape guard (and pay for it — see its doc comment).
func TestFunctionAnywhereIsSpecLayerOnly(t *testing.T) {
	const allowedDir = "internal/spec"

	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolving the repo root: %v", err)
	}
	fset := token.NewFileSet()
	scanned := 0
	err = filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			// Fixture projects are separate modules that never import this one,
			// and .git/node_modules are not source.
			case ".git", "node_modules", "testdata", "test_cgo_mixed", "test_cgo_demo":
				return fs.SkipDir
			}
			return nil
		}
		name := d.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		file, perr := parser.ParseFile(fset, path, nil, 0)
		if perr != nil {
			return fmt.Errorf("parsing %s: %w", path, perr)
		}
		scanned++
		rel, rerr := filepath.Rel(repoRoot, path)
		if rerr != nil {
			rel = path
		}
		allowed := strings.HasPrefix(filepath.ToSlash(rel), allowedDir+"/")
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			called := ""
			switch fn := call.Fun.(type) {
			case *ast.Ident:
				called = fn.Name
			case *ast.SelectorExpr:
				called = fn.Sel.Name
			}
			if called == "FunctionAnywhere" && !allowed {
				t.Errorf("%s calls FunctionAnywhere from outside %s; its memo is only "+
					"safe because every caller runs after metadata assembly has finished",
					fset.Position(call.Pos()), allowedDir)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking the repo: %v", err)
	}
	if scanned == 0 {
		t.Fatal("scanned no source files — the check would pass vacuously")
	}
}
