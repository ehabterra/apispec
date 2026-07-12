package generator

import (
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_InterfaceResponse locks in interface-typed response resolution:
// a handler that encodes an interface-typed variable resolves to the concrete
// type statically assigned to it, and falls back to the interface when the
// concrete type is ambiguous (more than one assigned).
func TestTestdata_InterfaceResponse(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "interface_response", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	schemas := out.Components.Schemas
	findSchema := func(suffix string) *intspec.Schema {
		for k, v := range schemas {
			if strings.HasSuffix(k, suffix) {
				return v
			}
		}
		return nil
	}
	// responseRef returns the trailing component name a path's POST response
	// resolves to.
	responseRef := func(path string) string {
		item, ok := out.Paths[path]
		if !ok {
			t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
		op := opFor(item, "POST")
		if op == nil {
			t.Fatalf("POST %s missing", path)
		}
		for _, resp := range op.Responses {
			for _, mt := range resp.Content {
				if mt.Schema != nil && mt.Schema.Ref != "" {
					return mt.Schema.Ref
				}
			}
		}
		return ""
	}

	// /dog: `var a Animal = Dog{}` → concrete Dog (with breed field).
	if ref := responseRef("/dog"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /dog response = %q, want the concrete Dog", ref)
	}
	if dog := findSchema("_response_Dog"); dog == nil || dog.Properties["breed"] == nil {
		t.Errorf("Dog schema missing 'breed'; got %+v", dog)
	}

	// /cat: `var a Animal; a = Cat{}` → concrete Cat (with lives field).
	if ref := responseRef("/cat"); !strings.HasSuffix(ref, "_Cat") {
		t.Errorf("POST /cat response = %q, want the concrete Cat", ref)
	}
	if cat := findSchema("_response_Cat"); cat == nil || cat.Properties["lives"] == nil {
		t.Errorf("Cat schema missing 'lives'; got %+v", cat)
	}

	// /either: two concrete types assigned → ambiguous → keep the Animal
	// interface (an empty-object schema), NOT one of the concretes.
	if ref := responseRef("/either"); !strings.HasSuffix(ref, "_Animal") {
		t.Errorf("POST /either response = %q, want the Animal interface (ambiguous concrete)", ref)
	}

	// /made: `Encode(makeDog())` where makeDog() Animal { return Dog{} } →
	// resolves to the concrete Dog via return-value tracing.
	if ref := responseRef("/made"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /made response = %q, want the concrete Dog (return trace)", ref)
	}

	// /passed: `writeAnimal(w, Dog{})` where writeAnimal(w, v Animal) encodes v
	// → resolves to Dog via the named-interface parameter binding.
	if ref := responseRef("/passed"); !strings.HasSuffix(ref, "_Dog") {
		t.Errorf("POST /passed response = %q, want the concrete Dog (param binding)", ref)
	}
}
