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

package insight

import (
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

func ref(name string) *spec.Schema {
	return &spec.Schema{Ref: refPrefix + name}
}

func jsonResp(status string, sc *spec.Schema) map[string]spec.Response {
	return map[string]spec.Response{
		status: {Content: map[string]spec.MediaType{"application/json": {Schema: sc}}},
	}
}

// A spec exercising the four issue classes plus a clean route.
func sampleSpec() *spec.OpenAPISpec {
	wrapper := &spec.Schema{AllOf: []*spec.Schema{
		ref("Env"),
		{Type: "object", Properties: map[string]*spec.Schema{"data": ref("Order")}},
	}}
	return &spec.OpenAPISpec{
		Paths: map[string]spec.PathItem{
			"/a": {Get: &spec.Operation{Tags: []string{"orders"}, Responses: jsonResp("200", ref("Order"))}},
			"/b": {Post: &spec.Operation{Responses: jsonResp("200", ref("Missing"))}},
			"/c": {Get: &spec.Operation{Responses: jsonResp("200", ref("Placeholder"))}},
			"/d": {Get: &spec.Operation{Tags: []string{"orders"}, Responses: jsonResp("200", wrapper)}},
		},
		Components: &spec.Components{Schemas: map[string]*spec.Schema{
			"Order":       {Type: "object"},
			"Env":         {Type: "object"},
			"Placeholder": {Type: "object", Description: "External or unresolved type: foo.Bar"},
		}},
	}
}

func TestBuildOverview_CountsAndHealth(t *testing.T) {
	rep := BuildOverview(sampleSpec(), nil)

	if rep.Routes != 4 || rep.Operations != 4 {
		t.Fatalf("routes/ops = %d/%d, want 4/4", rep.Routes, rep.Operations)
	}
	if rep.Components != 3 {
		t.Errorf("components = %d, want 3", rep.Components)
	}
	// methods: GET x3, POST x1
	if got := countOf(rep.ByMethod, "GET"); got != 3 {
		t.Errorf("GET = %d, want 3", got)
	}
	if got := countOf(rep.ByMethod, "POST"); got != 1 {
		t.Errorf("POST = %d, want 1", got)
	}
	// clean routes: /a and /d (info-only) → 2 of 4 → 50%
	if rep.Health.Score != 50 {
		t.Errorf("health = %d, want 50 (%+v)", rep.Health.Score, rep.Health)
	}
	// Order referenced by /a and /d(data) → top type with count 2
	if len(rep.TopTypes) == 0 || rep.TopTypes[0].Name != "Order" || rep.TopTypes[0].Count != 2 {
		t.Errorf("top type = %+v, want Order x2", rep.TopTypes)
	}
}

func TestBuildOverview_Issues(t *testing.T) {
	rep := BuildOverview(sampleSpec(), nil)

	want := map[string]string{
		"dangling-ref":        "Missing",     // /b
		"unresolved-type":     "Placeholder", // /c
		"wrapper-specialised": "",            // /d (info)
	}
	got := map[string]bool{}
	for _, is := range rep.Issues {
		got[is.Kind] = true
		if exp, ok := want[is.Kind]; ok && exp != "" && is.Ref != exp {
			t.Errorf("issue %s ref = %q, want %q", is.Kind, is.Ref, exp)
		}
	}
	for kind := range want {
		if !got[kind] {
			t.Errorf("missing expected issue kind %q; issues=%+v", kind, rep.Issues)
		}
	}
	// warns sort before info
	if rep.Issues[0].Severity != "warn" {
		t.Errorf("first issue should be a warning, got %+v", rep.Issues[0])
	}
}

func TestBuildOverview_NoResponses(t *testing.T) {
	s := &spec.OpenAPISpec{Paths: map[string]spec.PathItem{
		"/x": {Get: &spec.Operation{Responses: map[string]spec.Response{}}},
	}}
	rep := BuildOverview(s, nil)
	if !hasKind(rep.Issues, "no-responses") {
		t.Errorf("expected no-responses issue, got %+v", rep.Issues)
	}
	if rep.Health.Score != 0 {
		t.Errorf("health = %d, want 0 (the only route is unclean)", rep.Health.Score)
	}
}

// Regression: a clean spec (no issues) must still return a non-nil
// Issues slice so it marshals as [] not null (the UI does .filter on it).
func TestBuildOverview_IssuesNeverNil(t *testing.T) {
	s := &spec.OpenAPISpec{
		Paths: map[string]spec.PathItem{
			"/ok": {Get: &spec.Operation{Responses: jsonResp("200", &spec.Schema{Type: "object"})}},
		},
	}
	rep := BuildOverview(s, nil)
	if rep.Issues == nil {
		t.Fatal("Issues must be non-nil (would marshal as JSON null)")
	}
	if len(rep.Issues) != 0 {
		t.Errorf("expected 0 issues, got %+v", rep.Issues)
	}
}

func TestBuildOverview_ResolutionCoverageTaxonomy(t *testing.T) {
	s := &spec.OpenAPISpec{
		Paths: map[string]spec.PathItem{
			// clean write op: has a body + a documented 400 → full, body+error covered
			"/a": {Post: &spec.Operation{
				RequestBody: &spec.RequestBody{Content: map[string]spec.MediaType{"application/json": {Schema: ref("In")}}},
				Responses: map[string]spec.Response{
					"201": {Content: map[string]spec.MediaType{"application/json": {Schema: ref("Out")}}},
					"400": {},
				},
			}},
			// only 200 + default → partial (default-status), no documented error
			"/b": {Get: &spec.Operation{Responses: map[string]spec.Response{
				"200":     {Content: map[string]spec.MediaType{"application/json": {Schema: ref("Out")}}},
				"default": {},
			}}},
			// dangling ref → broken
			"/c": {Get: &spec.Operation{Responses: jsonResp("200", ref("Missing"))}},
		},
		Components: &spec.Components{Schemas: map[string]*spec.Schema{"In": {Type: "object"}, "Out": {Type: "object"}}},
	}
	rep := BuildOverview(s, nil)

	if rep.Resolution.Full != 1 || rep.Resolution.Partial != 1 || rep.Resolution.Broken != 1 {
		t.Errorf("resolution = %+v, want full1 partial1 broken1", rep.Resolution)
	}
	if rep.Coverage.RequestBody.Have != 1 || rep.Coverage.RequestBody.Total != 1 {
		t.Errorf("request-body coverage = %+v, want 1/1", rep.Coverage.RequestBody)
	}
	if rep.Coverage.ErrorResponses.Have != 1 || rep.Coverage.ErrorResponses.Total != 3 {
		t.Errorf("error-response coverage = %+v, want 1/3", rep.Coverage.ErrorResponses)
	}
	if countOf(rep.Taxonomy, "default-status") != 1 {
		t.Errorf("taxonomy default-status = %d, want 1 (%+v)", countOf(rep.Taxonomy, "default-status"), rep.Taxonomy)
	}
	if countOf(rep.Taxonomy, "dangling-ref") != 1 {
		t.Errorf("taxonomy dangling-ref = %d, want 1 (%+v)", countOf(rep.Taxonomy, "dangling-ref"), rep.Taxonomy)
	}
	// default-status is info-level, so /b stays "clean"; health counts /a and /b.
	if rep.Health.Score != 67 {
		t.Errorf("health = %d, want 67 (default-status must not drop the score)", rep.Health.Score)
	}
}

func TestInterfaceStats(t *testing.T) {
	sp := metadata.NewStringPool()
	iface := sp.Get("interface")
	mkIface := func(name string, impls int) *metadata.Type {
		by := make([]int, impls)
		for i := range by {
			by[i] = sp.Get(name + "Impl" + string(rune('a'+i)))
		}
		return &metadata.Type{Name: sp.Get(name), Pkg: sp.Get("pkg"), Kind: iface, ImplementedBy: by}
	}
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"pkg": {Files: map[string]*metadata.File{
				"f.go": {Types: map[string]*metadata.Type{
					"Store": mkIface("Store", 2), // ambiguous
					"Clock": mkIface("Clock", 1), // single
					"Empty": mkIface("Empty", 0), // unimplemented
					"Foo":   {Name: sp.Get("Foo"), Pkg: sp.Get("pkg"), Kind: sp.Get("struct")},
				}},
			}},
		},
	}
	st := interfaceStats(meta)
	if st.Total != 3 || st.SingleImpl != 1 || st.Ambiguous != 1 || st.Unimplemented != 1 {
		t.Errorf("interface stats = %+v, want total3 single1 ambiguous1 unimpl1", st)
	}
	if len(st.AmbiguousList) != 1 || st.AmbiguousList[0].Name != "pkg.Store" || st.AmbiguousList[0].Count != 2 {
		t.Errorf("ambiguous list = %+v, want [pkg.Store:2]", st.AmbiguousList)
	}
	if interfaceStats(nil).Total != 0 {
		t.Error("nil meta should be zero stats")
	}
}

// Two ambiguous interfaces sharing a bare name in different packages must both
// survive as distinct, package-qualified entries — a bare-name ambig key would
// let one overwrite the other, and map-iteration order would decide which,
// producing non-deterministic output (golden rule #1).
func TestInterfaceStats_SameNameDifferentPackages(t *testing.T) {
	sp := metadata.NewStringPool()
	iface := sp.Get("interface")
	mkIface := func(pkg, name string, impls int) *metadata.Type {
		by := make([]int, impls)
		for i := range by {
			by[i] = sp.Get(pkg + name + "Impl" + string(rune('a'+i)))
		}
		return &metadata.Type{Name: sp.Get(name), Pkg: sp.Get(pkg), Kind: iface, ImplementedBy: by}
	}
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"github.com/me/app/orders": {Types: map[string]*metadata.Type{
				"Store": mkIface("github.com/me/app/orders", "Store", 2),
			}},
			"github.com/me/app/users": {Types: map[string]*metadata.Type{
				"Store": mkIface("github.com/me/app/users", "Store", 3),
			}},
		},
	}
	st := interfaceStats(meta)
	if st.Ambiguous != 2 {
		t.Fatalf("ambiguous = %d, want 2 (both Store interfaces)", st.Ambiguous)
	}
	got := map[string]int{}
	for _, c := range st.AmbiguousList {
		got[c.Name] = c.Count
	}
	if got["orders.Store"] != 2 || got["users.Store"] != 3 {
		t.Errorf("ambiguous list = %+v, want orders.Store:2 users.Store:3", st.AmbiguousList)
	}
}

func TestVerbDispatch(t *testing.T) {
	sp := metadata.NewStringPool()
	pkg := "github.com/x/httpapi"
	meta := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			pkg: {Files: map[string]*metadata.File{
				"h.go": {Functions: map[string]*metadata.Function{
					"Widget": {Name: sp.Get("Widget"), Pkg: sp.Get(pkg),
						MethodDispatch: []metadata.MethodBranch{{Methods: []string{"POST"}}, {Methods: []string{"GET"}}}},
					"Plain":  {Name: sp.Get("Plain"), Pkg: sp.Get(pkg)},
					"Single": {Name: sp.Get("Single"), Pkg: sp.Get(pkg), MethodDispatch: []metadata.MethodBranch{{Methods: []string{"GET"}}}},
				}},
			}},
		},
	}
	vd := verbDispatch(meta)
	if len(vd) != 1 {
		t.Fatalf("verb dispatch = %+v, want exactly 1 (multi-method only)", vd)
	}
	if vd[0].Handler != "httpapi.Widget" {
		t.Errorf("handler = %q, want httpapi.Widget", vd[0].Handler)
	}
	if len(vd[0].Methods) != 2 || vd[0].Methods[0] != "GET" || vd[0].Methods[1] != "POST" {
		t.Errorf("methods = %v, want sorted [GET POST]", vd[0].Methods)
	}
}

func TestSchemaRefs_DedupAndNested(t *testing.T) {
	s := &spec.Schema{
		Properties: map[string]*spec.Schema{
			"a": ref("X"),
			"b": {Items: ref("X")}, // duplicate X
			"c": ref("Y"),
		},
	}
	refs := schemaRefs(s)
	if len(refs) != 2 {
		t.Fatalf("refs = %v, want 2 unique (X,Y)", refs)
	}
}

func TestIsWrapperSpecialised(t *testing.T) {
	yes := &spec.Schema{AllOf: []*spec.Schema{ref("Env"), {Type: "object", Properties: map[string]*spec.Schema{"data": ref("O")}}}}
	if !isWrapperSpecialised(yes) {
		t.Error("expected wrapper-specialised true")
	}
	no := &spec.Schema{AllOf: []*spec.Schema{ref("Env"), ref("Other")}}
	if isWrapperSpecialised(no) {
		t.Error("two refs is not a specialisation")
	}
	if isWrapperSpecialised(ref("Env")) {
		t.Error("single ref is not a specialisation")
	}
}

func TestCallGraphStats(t *testing.T) {
	if st := callGraphStats(nil); st.Packages != 0 {
		t.Errorf("nil meta should be zero stats, got %+v", st)
	}
	meta := &metadata.Metadata{
		Packages: map[string]*metadata.Package{
			"p": {Files: map[string]*metadata.File{
				"f.go": {Functions: map[string]*metadata.Function{"fn": {}, "fn2": {}}},
			}},
		},
		CallGraph: []metadata.CallGraphEdge{{}, {}, {}},
	}
	st := callGraphStats(meta)
	if st.Packages != 1 || st.Functions != 2 || st.Edges != 3 {
		t.Errorf("stats = %+v, want pkgs1 fns2 edges3", st)
	}
}

func TestBuildExportMarkdown(t *testing.T) {
	rep := BuildOverview(sampleSpec(), nil)
	md := BuildExportMarkdown(rep, ExportOptions{ConfigYAML: "framework: chi\n"})
	for _, want := range []string{
		"# apispec — issues to resolve",
		"POST /b",
		"dangling-ref",
		"GET /c",
		"## Current apispec config",
		"framework: chi",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("export markdown missing %q\n---\n%s", want, md)
		}
	}
}

func TestBuildExportMarkdown_Redact(t *testing.T) {
	rep := &OverviewReport{
		Issues: []Issue{{Severity: "warn", Kind: "dangling-ref", Method: "GET", Path: "/x", Ref: "github.com/me/app_pkg_Foo", Detail: "github.com/me/app thing"}},
	}
	md := BuildExportMarkdown(rep, ExportOptions{ModulePath: "github.com/me/app", Redact: true})
	if strings.Contains(md, "github.com/me/app") {
		t.Errorf("redaction failed; module path leaked:\n%s", md)
	}
	if !strings.Contains(md, "example.com/app") {
		t.Errorf("expected redaction placeholder; got:\n%s", md)
	}
}

// helpers
func countOf(cs []Count, name string) int {
	for _, c := range cs {
		if c.Name == name {
			return c.Count
		}
	}
	return -1
}
func hasKind(issues []Issue, kind string) bool {
	for _, i := range issues {
		if i.Kind == kind {
			return true
		}
	}
	return false
}
