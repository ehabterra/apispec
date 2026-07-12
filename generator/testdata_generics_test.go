package generator

import (
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// TestTestdata_GenericStructs locks in resolution of parametric (generic)
// response envelopes returned directly at the encode site — the Page[T] /
// Envelope[T] pattern. Before this was supported, every instantiation
// collapsed onto a single placeholder schema (Page_T-any) whose payload
// pointed at an empty T-any object, and the concrete argument structs were
// never emitted. Now each concrete instantiation must resolve to its own
// component with the type argument substituted into the parametric field:
//   - Page[User]    -> items: array of $ref User
//   - Page[Product] -> items: array of $ref Product (distinct from Page[User])
//   - Envelope[User]-> data:  $ref User
//
// and User / Product must be emitted as their own component schemas.
func TestTestdata_GenericStructs(t *testing.T) {
	out := loadTestdataWithFixtureConfig(t, "generic_structs", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)
	noUnresolvedPlaceholders(t, out)

	for _, p := range []string{"/users", "/products", "/user"} {
		item, ok := out.Paths[p]
		if !ok {
			t.Fatalf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
		if opFor(item, "POST") == nil {
			t.Errorf("POST %s: missing operation", p)
		}
	}

	schemas := out.Components.Schemas

	// findSchema locates a component whose key ends with the given suffix. Keys
	// are the sanitized fully-qualified names (…generic_structs_Page_User).
	findSchema := func(suffix string) (string, *intspec.Schema) {
		for k, v := range schemas {
			if strings.HasSuffix(k, suffix) {
				return k, v
			}
		}
		return "", nil
	}

	// The concrete argument structs must exist as their own components.
	_, userSchema := findSchema("_structs_User")
	if userSchema == nil {
		t.Fatalf("User component missing; have %v", mapSchemaKeys(schemas))
	}
	if _, ok := userSchema.Properties["email"]; !ok {
		t.Errorf("User schema missing expected field 'email'; got %v", userSchema.Properties)
	}
	_, productSchema := findSchema("_structs_Product")
	if productSchema == nil {
		t.Fatalf("Product component missing; have %v", mapSchemaKeys(schemas))
	}
	if _, ok := productSchema.Properties["sku"]; !ok {
		t.Errorf("Product schema missing expected field 'sku'; got %v", productSchema.Properties)
	}

	// Page[User]: distinct component, items -> array of $ref User.
	pageUserKey, pageUser := findSchema("_structs_Page_User")
	if pageUser == nil {
		t.Fatalf("Page_User component missing; have %v", mapSchemaKeys(schemas))
	}
	items := pageUser.Properties["items"]
	if items == nil || items.Type != "array" || items.Items == nil {
		t.Fatalf("Page_User.items = %+v, want an array schema", items)
	}
	if !strings.HasSuffix(items.Items.Ref, "_User") {
		t.Errorf("Page_User.items.items ref = %q, want a $ref to the User component", items.Items.Ref)
	}

	// Page[Product]: distinct component with Product elements — proves the two
	// Page instantiations do NOT collapse onto one schema.
	pageProductKey, pageProduct := findSchema("_structs_Page_Product")
	if pageProduct == nil {
		t.Fatalf("Page_Product component missing; have %v", mapSchemaKeys(schemas))
	}
	if pageProductKey == pageUserKey {
		t.Fatalf("Page[User] and Page[Product] collapsed onto one schema key %q", pageUserKey)
	}
	pItems := pageProduct.Properties["items"]
	if pItems == nil || pItems.Items == nil || !strings.HasSuffix(pItems.Items.Ref, "_Product") {
		t.Errorf("Page_Product.items.items = %+v, want a $ref to the Product component", pItems)
	}

	// Envelope[User]: bare type-parameter payload — data -> $ref User.
	_, envUser := findSchema("_structs_Envelope_User")
	if envUser == nil {
		t.Fatalf("Envelope_User component missing; have %v", mapSchemaKeys(schemas))
	}
	if data := envUser.Properties["data"]; data == nil || !strings.HasSuffix(data.Ref, "_User") {
		t.Errorf("Envelope_User.data = %+v, want a $ref to the User component", data)
	}

	// No lingering unsubstituted placeholder (the pre-fix Page_T-any / T-any).
	for k := range schemas {
		if strings.HasSuffix(k, "_T-any") || strings.HasSuffix(k, "_T") {
			t.Errorf("found unsubstituted generic placeholder component %q", k)
		}
	}
}
