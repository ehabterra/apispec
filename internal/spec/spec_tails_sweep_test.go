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
)

// identArg builds an ident CallArgument with the given rendered name.
func identArg(m *metadata.Metadata, name string) *metadata.CallArgument {
	a := metadata.NewCallArgument(m)
	a.SetKind(metadata.KindIdent)
	a.SetName(name)
	return a
}

func TestWrapperSpecialisationHelpers(t *testing.T) {
	sp := metadata.NewStringPool()
	m := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"app": {
				Files: map[string]*metadata.File{
					"resp.go": {
						Types: map[string]*metadata.Type{
							"Envelope": {
								Name: sp.Get("Envelope"),
								Fields: []metadata.Field{
									{Name: sp.Get("Data"), Type: sp.Get("interface{}"), Tag: sp.Get(`json:"data"`)},
									{Name: sp.Get("Any"), Type: sp.Get("*any")},
									{Name: sp.Get("Code"), Type: sp.Get("int")},
								},
							},
						},
					},
				},
			},
		},
	}
	cp := NewContextProvider(m)

	if metadataFromContextProvider(cp) != m {
		t.Error("metadataFromContextProvider should unwrap the impl")
	}
	if metadataFromContextProvider(nil) != nil {
		t.Error("nil provider should yield nil metadata")
	}

	// paramNamesOf tolerates nil functions and nil args.
	if got := paramNamesOf(nil); got != nil {
		t.Errorf("paramNamesOf(nil) = %v", got)
	}
	fn := &metadata.Function{Signature: metadata.CallArgument{
		Kind: -1, Name: -1, Value: -1, Raw: -1, Pkg: -1, Type: -1, Position: -1, ResolvedType: -1, GenericTypeName: -1, Meta: m,
	}}
	fn.Signature.Args = []*metadata.CallArgument{identArg(m, "w"), nil, identArg(m, "data")}
	names := paramNamesOf(fn)
	if len(names) != 3 || names[0] != "w" || names[1] != "" || names[2] != "data" {
		t.Errorf("paramNamesOf = %v", names)
	}

	args := []*metadata.CallArgument{identArg(m, "a0"), identArg(m, "a1")}
	if got := lookupCallArgByParamName(args, names, "data"); got != nil {
		t.Error("index beyond call args must miss")
	}
	if got := lookupCallArgByParamName(args, names, "w"); got != args[0] {
		t.Error("param name should map to positional arg")
	}
	if got := lookupCallArgByParamName(args, names, "absent"); got != nil {
		t.Error("unknown param must miss")
	}

	// parentEdgeOf.
	edge := &metadata.CallGraphEdge{}
	parent := &TrackerNode{key: "p", CallGraphEdge: edge}
	child := &TrackerNode{key: "c", Parent: parent}
	if parentEdgeOf(child) != edge {
		t.Error("parentEdgeOf should return the parent's edge")
	}
	if parentEdgeOf(&TrackerNode{key: "orphan"}) != nil {
		t.Error("orphan node has no parent edge")
	}

	// lookupWrapperType via typeByName.
	if lookupWrapperType(nil, "app.Envelope") != nil {
		t.Error("nil meta must miss")
	}
	if lookupWrapperType(m, "") != nil {
		t.Error("empty type must miss")
	}
	wt := lookupWrapperType(m, "app.Envelope")
	if wt == nil {
		t.Fatal("Envelope not found")
	}

	// wrapperFieldIsGeneric: interface{} and *any are generic; int is not.
	if !wrapperFieldIsGeneric(m, wt, "Data") {
		t.Error("interface{} field should be generic")
	}
	if !wrapperFieldIsGeneric(m, wt, "Any") {
		t.Error("*any field should be generic")
	}
	if wrapperFieldIsGeneric(m, wt, "Code") {
		t.Error("int field should not be generic")
	}
	if wrapperFieldIsGeneric(m, wt, "Missing") || wrapperFieldIsGeneric(m, nil, "Data") {
		t.Error("missing field / nil type should not be generic")
	}

	// jsonNameForField: tag name, fallback to field name, missing field.
	if got := jsonNameForField(m, wt, "Data"); got != "data" {
		t.Errorf("json name = %q, want data", got)
	}
	if got := jsonNameForField(m, wt, "Code"); got != "Code" {
		t.Errorf("untagged json name = %q, want Code", got)
	}
	if got := jsonNameForField(m, wt, "Missing"); got != "" {
		t.Errorf("missing field json name = %q", got)
	}
	if got := jsonNameForField(m, nil, "Data"); got != "" {
		t.Errorf("nil wrapper json name = %q", got)
	}
}

func TestSecurityLookupAssignments(t *testing.T) {
	sp := metadata.NewStringPool()
	m := &metadata.Metadata{
		StringPool: sp,
		Packages: map[string]*metadata.Package{
			"app": {
				Files: map[string]*metadata.File{
					"main.go": {
						Functions: map[string]*metadata.Function{
							"setup": {
								Name: sp.Get("setup"),
								AssignmentMap: map[string][]metadata.Assignment{
									"authMW": {{
										VariableName: sp.Get("authMW"),
										CalleeFunc:   "RequireAuth",
										CalleePkg:    "app/auth",
										Value:        metadata.CallArgument{Kind: -1, Name: -1, Value: -1, Raw: -1, Pkg: -1, Type: -1, Position: -1, ResolvedType: -1, GenericTypeName: -1, Meta: nil},
									}},
								},
							},
						},
					},
				},
			},
		},
	}

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{Meta: m, Name: sp.Get("setup"), Pkg: sp.Get("app"), RecvType: -1},
		Callee: metadata.Call{Meta: m, Name: sp.Get("Use"), Pkg: sp.Get("gin"), RecvType: -1},
	}

	// Falls back to the caller function's assignment map.
	assigns := lookupAssignments(edge, "authMW", m)
	if len(assigns) != 1 || assigns[0].CalleeFunc != "RequireAuth" {
		t.Fatalf("caller-fallback assignments = %+v", assigns)
	}

	// Edge's own map wins when present.
	edge.AssignmentMap = map[string][]metadata.Assignment{
		"authMW": {{CalleeFunc: "EdgeLocal"}},
	}
	if got := lookupAssignments(edge, "authMW", m); len(got) != 1 || got[0].CalleeFunc != "EdgeLocal" {
		t.Errorf("edge-local assignments = %+v", got)
	}

	// Unknown variable and nil meta.
	if got := lookupAssignments(edge, "nothing", nil); got != nil {
		t.Errorf("nil meta = %+v", got)
	}

	// resolveMiddlewareIdentRef end-to-end over the CalleeFunc path.
	edge.AssignmentMap = nil
	arg := identArg(m, "authMW")
	ref, ok := resolveMiddlewareIdentRef(edge, arg, m)
	if !ok || ref.FunctionName != "RequireAuth" || ref.Pkg != "app/auth" {
		t.Errorf("middleware ref = %+v ok=%v", ref, ok)
	}
	// Non-ident and missing-variable rejections.
	lit := metadata.NewCallArgument(m)
	lit.SetKind(metadata.KindLiteral)
	if _, ok := resolveMiddlewareIdentRef(edge, lit, m); ok {
		t.Error("literal must not resolve")
	}
	if _, ok := resolveMiddlewareIdentRef(edge, identArg(m, "unknown"), m); ok {
		t.Error("unknown ident must not resolve")
	}
}

func TestTrackerMetadataInterfaceResolution(t *testing.T) {
	sp := metadata.NewStringPool()
	m := &metadata.Metadata{StringPool: sp}
	m.RegisterInterfaceResolution("Storer", "Service", "app", "PgStore", "svc.go:10")

	tr := &TrackerTree{meta: m, interfaceResolutionMap: map[interfaceKey]string{}}

	// Cache miss resolves from metadata and caches locally.
	if got := tr.ResolveInterfaceFromMetadata("Storer", "Service", "app"); got != "PgStore" {
		t.Errorf("metadata resolution = %q, want PgStore", got)
	}
	// Now the local cache serves it.
	if got := tr.ResolveInterface("Storer", "Service", "app"); got != "PgStore" {
		t.Errorf("cached resolution = %q, want PgStore", got)
	}
	// Unknown stays itself.
	if got := tr.ResolveInterfaceFromMetadata("Other", "Service", "app"); got != "Other" {
		t.Errorf("unknown resolution = %q", got)
	}

	// SyncInterfaceResolutionsFromMetadata copies the metadata view in bulk.
	tr2 := &TrackerTree{meta: m, interfaceResolutionMap: map[interfaceKey]string{}}
	tr2.SyncInterfaceResolutionsFromMetadata()
	if got := tr2.ResolveInterface("Storer", "Service", "app"); got != "PgStore" {
		t.Errorf("synced resolution = %q, want PgStore", got)
	}
	// Nil metadata is a no-op.
	(&TrackerTree{interfaceResolutionMap: map[interfaceKey]string{}}).SyncInterfaceResolutionsFromMetadata()
}
