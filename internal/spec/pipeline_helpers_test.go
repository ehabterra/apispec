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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/typemodel"
)

// quoted wraps a value so extractConstantValue sees a fmt.Stringer.
type stringerVal string

func (s stringerVal) String() string { return string(s) }

func TestExtractConstantValue(t *testing.T) {
	cases := []struct {
		name string
		in   interface{}
		want interface{}
	}{
		{"nil", nil, nil},
		{"quoted string drops quotes", stringerVal(`"active"`), "active"},
		{"integer parses", stringerVal("42"), int64(42)},
		{"float parses", stringerVal("2.5"), 2.5},
		{"bool parses", stringerVal("true"), true},
		{"plain word stays string", stringerVal("pending"), "pending"},
		{"non-stringer passes through", 7, 7},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := extractConstantValue(c.in); got != c.want {
				t.Errorf("extractConstantValue(%v) = %v (%T), want %v (%T)", c.in, got, got, c.want, c.want)
			}
		})
	}
}

func TestTypeMatches(t *testing.T) {
	sp := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: sp, Packages: map[string]*metadata.Package{}}

	cases := []struct {
		name             string
		constant, target string
		want             bool
	}{
		{"direct", "Status", "Status", true},
		{"pointer constant", "*Status", "Status", true},
		{"pointer target", "Status", "*Status", true},
		{"both qualified same name", "app.Status", "other.Status", true},
		{"constant qualified", "app.Status", "Status", true},
		{"target qualified", "Status", "app.Status", true},
		{"mismatch", "Status", "Role", false},
		{"qualified mismatch", "app.Status", "app.Role", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := typeMatches(c.constant, c.target, meta); got != c.want {
				t.Errorf("typeMatches(%q, %q) = %v, want %v", c.constant, c.target, got, c.want)
			}
		})
	}
}

func TestGenericArgText(t *testing.T) {
	if got := genericArgText(nil); got != "" {
		t.Errorf("nil = %q", got)
	}
	if got := genericArgText(typemodel.Parse("main.User")); got != "main.User" {
		t.Errorf("raw-backed ref = %q", got)
	}
	// A ref built programmatically has no raw text and falls back to Simple().
	built := &typemodel.TypeRef{Kind: typemodel.KindNamed, Pkg: "main", Name: "User"}
	if got := genericArgText(built); got != "User" {
		t.Errorf("built ref = %q, want User (Simple)", got)
	}
}

func TestExtractValidationConstraints(t *testing.T) {
	if got := extractValidationConstraints(""); got != nil {
		t.Errorf("empty tag = %+v, want nil", got)
	}

	c := extractValidationConstraints(`validate:"required,email,min=5,max=10"`)
	if c == nil {
		t.Fatal("nil constraints")
	}
	if !c.Required {
		t.Error("required not set")
	}
	if c.Format != "email" {
		t.Errorf("format = %q", c.Format)
	}
	if c.Min == nil || *c.Min != 5 || c.Max == nil || *c.Max != 10 {
		t.Errorf("min/max = %v/%v", c.Min, c.Max)
	}

	c = extractValidationConstraints(`validate:"len=8"`)
	if c.MinLength == nil || *c.MinLength != 8 || c.MaxLength == nil || *c.MaxLength != 8 {
		t.Errorf("len= should pin both lengths, got %v/%v", c.MinLength, c.MaxLength)
	}

	c = extractValidationConstraints(`validate:"minlen=2,maxlen=6,url"`)
	if c.MinLength == nil || *c.MinLength != 2 || c.MaxLength == nil || *c.MaxLength != 6 {
		t.Errorf("minlen/maxlen = %v/%v", c.MinLength, c.MaxLength)
	}
	if c.Format != "uri" {
		t.Errorf("url format = %q", c.Format)
	}

	c = extractValidationConstraints(`validate:"oneof=red green blue"`)
	if len(c.Enum) != 3 || c.Enum[0] != "red" || c.Enum[2] != "blue" {
		t.Errorf("oneof enum = %v", c.Enum)
	}

	for tag, wantPattern := range map[string]string{
		`validate:"alpha"`:    `^[a-zA-Z]+$`,
		`validate:"alphanum"`: `^[a-zA-Z0-9]+$`,
		`validate:"numeric"`:  `^[0-9]+$`,
	} {
		if c := extractValidationConstraints(tag); c.Pattern != wantPattern {
			t.Errorf("%s pattern = %q, want %q", tag, c.Pattern, wantPattern)
		}
	}

	if c := extractValidationConstraints(`validate:"uuid"`); c.Format != "uuid" {
		t.Errorf("uuid format = %q", c.Format)
	}
}

func TestIsPrimitiveShapedSchema(t *testing.T) {
	cases := []struct {
		name string
		s    *Schema
		want bool
	}{
		{"nil", nil, false},
		{"ref", &Schema{Ref: "#/x"}, false},
		{"object with props", &Schema{Type: "object", Properties: map[string]*Schema{"a": {}}}, false},
		{"allOf wrapper", &Schema{AllOf: []*Schema{{}}}, false},
		{"map-like", &Schema{Type: "object", AdditionalProperties: &Schema{}}, false},
		{"bare object", &Schema{Type: "object"}, false},
		{"typeless", &Schema{}, false},
		{"string", &Schema{Type: "string"}, true},
		{"integer", &Schema{Type: "integer"}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isPrimitiveShapedSchema(c.s); got != c.want {
				t.Errorf("= %v, want %v", got, c.want)
			}
		})
	}
}

func TestTrackerStringers(t *testing.T) {
	wants := map[ArgumentType]string{
		ArgTypeDirectCallee: "DirectCallee",
		ArgTypeFunctionCall: "FunctionCall",
		ArgTypeVariable:     "Variable",
		ArgTypeLiteral:      "Literal",
		ArgTypeSelector:     "Selector",
		ArgTypeUnary:        "Unary",
		ArgTypeBinary:       "Binary",
		ArgTypeIndex:        "Index",
		ArgTypeComposite:    "Composite",
		ArgTypeTypeAssert:   "TypeAssert",
		ArgTypeComplex:      "Complex",
	}
	for at, want := range wants {
		if got := at.String(); got != want {
			t.Errorf("ArgumentType(%d).String() = %q, want %q", at, got, want)
		}
	}

	ak := assignmentKey{Name: "n", Pkg: "p", Type: "t", Container: "c"}
	if got := ak.String(); got != "ptnc" {
		t.Errorf("assignmentKey.String() = %q, want ptnc", got)
	}
	ik := interfaceKey{InterfaceType: "I", StructType: "S", Pkg: "P"}
	if got := ik.String(); got != "PSI" {
		t.Errorf("interfaceKey.String() = %q, want PSI", got)
	}
}

func TestRegisterAndResolveInterface(t *testing.T) {
	tr := &TrackerTree{interfaceResolutionMap: map[interfaceKey]string{}}
	tr.RegisterInterfaceResolution("Storer", "Service", "app", "PgStore")
	if got := tr.ResolveInterface("Storer", "Service", "app"); got != "PgStore" {
		t.Errorf("registered resolution = %q, want PgStore", got)
	}
	if got := tr.ResolveInterface("Other", "Service", "app"); got != "Other" {
		t.Errorf("unregistered resolution = %q, want the interface itself", got)
	}
}

func TestIsValidHTTPMethodAndGetPattern(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	cp := NewContextProvider(meta)
	cfg := &APISpecConfig{}

	rp := RoutePattern{CallRegex: "Get"}
	rm := NewRoutePatternMatcher(rp, cfg, cp)
	if !rm.isValidHTTPMethod("get") || rm.isValidHTTPMethod("FETCH") {
		t.Error("isValidHTTPMethod misclassifies")
	}
	if rm.GetPattern() == nil {
		t.Error("route GetPattern nil")
	}

	pm := NewParamPatternMatcher(ParamPattern{ParamIn: "path"}, cfg, cp)
	if pm.GetPattern() == nil {
		t.Error("param GetPattern nil")
	}
}

// TestFindTargetNodeAndRouterAssignment covers the mount router-assignment
// path: BFS lookup of the assignment's node, then traversal of its children.
func TestFindTargetNodeAndRouterAssignment(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	limits := metadata.TrackerLimits{MaxNodesPerTree: 100, MaxChildrenPerNode: 10, MaxArgsPerFunction: 5, MaxNestedArgsDepth: 3}
	tree := NewMockTrackerTree(meta, limits)

	child := &TrackerNode{key: "main.newRouter.child"}
	target := &TrackerNode{key: "main.newRouter", Children: []*TrackerNode{child}}
	root := &TrackerNode{key: "main.main", Children: []*TrackerNode{target}}
	tree.AddRoot(root)

	cfg := &APISpecConfig{}
	ex := NewExtractor(tree, cfg)

	assignment := metadata.NewCallArgument(meta)
	assignment.SetKind(metadata.KindIdent)
	assignment.SetName("main.newRouter")

	if ex.findTargetNode(nil) != nil {
		t.Error("nil assignment must find nothing")
	}
	found := ex.findTargetNode(assignment)
	if found == nil || found.GetKey() != "main.newRouter" {
		t.Fatalf("findTargetNode = %v, want main.newRouter", found)
	}

	missing := metadata.NewCallArgument(meta)
	missing.SetKind(metadata.KindIdent)
	missing.SetName("main.absent")
	if ex.findTargetNode(missing) != nil {
		t.Error("absent key must find nothing")
	}

	// handleRouterAssignment walks the found node's children; with a plain
	// child (no edge) it must not panic and must not add routes.
	var routes []*RouteInfo
	visited := map[string]bool{}
	ex.handleRouterAssignment(MountInfo{Assignment: assignment}, "/api", nil, nil, nil, &routes, visited)
	if len(routes) != 0 {
		t.Errorf("bare children produced %d routes", len(routes))
	}
	// Missing target is a no-op.
	ex.handleRouterAssignment(MountInfo{Assignment: missing}, "/api", nil, nil, nil, &routes, visited)
}
