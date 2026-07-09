package spec

import (
	"testing"
)

// TestDynamicParamNameRoundTrip pins the PascalCase + "Param" mangling so
// dynamicParamComponentName / refTargetName stay symmetric. ensureAllPathParams
// relies on this symmetry to recognise that a $ref covers a placeholder.
func TestDynamicParamNameRoundTrip(t *testing.T) {
	cases := []string{"mountPoint", "prefix", "x", "userID"}
	for _, name := range cases {
		ref := dynamicParamRef(name)
		got := refTargetName(ref)
		if got != name {
			t.Errorf("round-trip for %q: ref=%q decoded=%q", name, ref, got)
		}
	}
}

// TestAppendDynamicParamRefs_AddsAndDedupes ensures $ref entries are appended
// only once even when the same placeholder shows up in multiple sources, and
// that an inline param with the same name suppresses the $ref.
func TestAppendDynamicParamRefs_AddsAndDedupes(t *testing.T) {
	// Empty start: two dynamics produce two $refs.
	out := appendDynamicParamRefs(nil, []string{"mountPoint", "mountPoint", "version"})
	if len(out) != 2 {
		t.Fatalf("expected 2 params, got %d", len(out))
	}
	if out[0].Ref != "#/components/parameters/MountPointParam" {
		t.Errorf("first ref %q", out[0].Ref)
	}
	if out[1].Ref != "#/components/parameters/VersionParam" {
		t.Errorf("second ref %q", out[1].Ref)
	}

	// An inline param with matching name suppresses the $ref.
	existing := []Parameter{{Name: "mountPoint", In: "path", Required: true, Schema: &Schema{Type: "string"}}}
	out = appendDynamicParamRefs(existing, []string{"mountPoint"})
	if len(out) != 1 {
		t.Fatalf("expected inline to suppress $ref, got %d params", len(out))
	}
	if out[0].Ref != "" {
		t.Errorf("expected inline param preserved, got ref %q", out[0].Ref)
	}
}

// TestEnsureAllPathParams_RecognisesRef checks that a $ref-only entry counts
// as "covered" so ensureAllPathParams doesn't add a duplicate inline declaration
// for the same {name} in the path. Without this, the spec would carry both a
// $ref and an x-warning inline param for every dynamic placeholder.
func TestEnsureAllPathParams_RecognisesRef(t *testing.T) {
	params := []Parameter{{Ref: "#/components/parameters/MountPointParam"}}
	got := ensureAllPathParams("/{mountPoint}/{id}", params, nil)

	// Expect: original $ref kept, plus one inline for {id}. No duplicate for mountPoint.
	if len(got) != 2 {
		t.Fatalf("expected 2 params (ref + id), got %d: %+v", len(got), got)
	}
	var sawRef, sawID bool
	for _, p := range got {
		if p.Ref == "#/components/parameters/MountPointParam" {
			sawRef = true
		}
		if p.Name == "id" {
			sawID = true
		}
	}
	if !sawRef || !sawID {
		t.Errorf("expected to see both $ref and inline id: %+v", got)
	}
}

// TestAddDynamicPathParamComponents_RegistersOnce ensures each unique
// placeholder appears exactly once in components.parameters, even when many
// routes share the same name (the typical case: every operation under a
// dynamic-mount prefix references the same placeholder).
func TestAddDynamicPathParamComponents_RegistersOnce(t *testing.T) {
	routes := []*RouteInfo{
		{DynamicParams: []string{"mountPoint"}},
		{DynamicParams: []string{"mountPoint"}},
		{DynamicParams: []string{"mountPoint", "version"}},
	}
	var components Components
	addDynamicPathParamComponents(&components, routes)

	if components.Parameters == nil {
		t.Fatal("expected components.Parameters populated")
	}
	if len(components.Parameters) != 2 {
		t.Fatalf("expected 2 unique components, got %d", len(components.Parameters))
	}
	mp, ok := components.Parameters["MountPointParam"]
	if !ok || mp.Name != "mountPoint" || mp.In != "path" || !mp.Required {
		t.Errorf("MountPointParam missing or malformed: %+v", mp)
	}
	if _, ok := components.Parameters["VersionParam"]; !ok {
		t.Errorf("VersionParam missing")
	}
}
