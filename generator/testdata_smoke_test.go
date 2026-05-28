package generator

import (
	"path/filepath"
	"strings"
	"testing"

	intspec "github.com/ehabterra/apispec/internal/spec"
	"github.com/ehabterra/apispec/spec"
)

// testdata fixtures live two levels up from this test file (under
// repo-root/testdata/<name>), so every load goes through this helper.
func loadTestdata(t *testing.T, name string, cfg *spec.APISpecConfig) *spec.OpenAPISpec {
	t.Helper()
	dir := filepath.Join("..", "testdata", name)
	out, err := NewGenerator(cfg).GenerateFromDirectory(dir)
	if err != nil {
		t.Fatalf("GenerateFromDirectory(%s) failed: %v", dir, err)
	}
	if out == nil || out.Paths == nil {
		t.Fatalf("nil spec or paths for %s", name)
	}
	return out
}

// firstRequestSchema returns the request body schema attached to the
// first operation of the given path, or nil if no request body is
// declared.
func firstRequestSchema(t *testing.T, out *spec.OpenAPISpec, path string) *intspec.Schema {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
	}
	op := firstOperation(&item)
	if op == nil || op.RequestBody == nil {
		return nil
	}
	for _, media := range op.RequestBody.Content {
		if media.Schema != nil {
			return media.Schema
		}
	}
	return nil
}

func hasPath(out *spec.OpenAPISpec, path string) bool {
	_, ok := out.Paths[path]
	return ok
}

// componentByName returns the first component schema whose key ends
// with suffix. This avoids hard-coding the full sanitised key, which
// depends on the module path.
func componentByName(out *spec.OpenAPISpec, suffix string) *intspec.Schema {
	if out.Components == nil {
		return nil
	}
	for k, v := range out.Components.Schemas {
		if strings.HasSuffix(k, suffix) {
			return v
		}
	}
	return nil
}

// noUnresolvedPlaceholders asserts that no component schema carries
// the "External or unresolved type" description — a strong sign that
// a body type leaked through as an opaque string instead of being
// resolved.
func noUnresolvedPlaceholders(t *testing.T, out *spec.OpenAPISpec) {
	t.Helper()
	if out.Components == nil {
		return
	}
	for k, v := range out.Components.Schemas {
		if v == nil {
			continue
		}
		if strings.Contains(v.Description, "External or unresolved type") {
			t.Errorf("unresolved-type placeholder leaked into components: %s -> %q", k, v.Description)
		}
	}
}

// ---------------------------------------------------------------------
// anonymous_struct
// ---------------------------------------------------------------------

// Locks in the metadata-side fix for inline `var req struct{...}`
// bodies: each request body is an inline `object` with real properties,
// and named field types are promoted to components.
func TestTestdata_AnonymousStruct(t *testing.T) {
	out := loadTestdata(t, "anonymous_struct", spec.DefaultChiConfig())

	for _, p := range []string{"/orders", "/bulk-update", "/tags", "/summary"} {
		if !hasPath(out, p) {
			t.Errorf("missing path %q; have %v", p, mapPathKeys(out.Paths))
		}
	}

	// /orders requestBody is an inline object whose `items` is an
	// array of $ref(itemReq).
	rb := firstRequestSchema(t, out, "/orders")
	if rb == nil || rb.Type != "object" || rb.Ref != "" {
		t.Fatalf("/orders requestBody should be inline object, got %+v", rb)
	}
	items := rb.Properties["items"]
	if items == nil || items.Type != "array" || items.Items == nil || items.Items.Ref == "" {
		t.Fatalf("/orders items should be array of $ref, got %+v", items)
	}

	// /bulk-update has a nested anonymous struct (`meta`) that must
	// stay inline.
	rb = firstRequestSchema(t, out, "/bulk-update")
	if rb == nil || rb.Type != "object" {
		t.Fatalf("/bulk-update requestBody should be object, got %+v", rb)
	}
	meta := rb.Properties["meta"]
	if meta == nil || meta.Type != "object" || meta.Ref != "" {
		t.Fatalf("nested anonymous struct must inline, got %+v", meta)
	}

	for _, name := range []string{"itemReq", "summaryStat", "updateOp"} {
		if componentByName(out, name) == nil {
			t.Errorf("expected component ending in %q; have %v",
				name, mapSchemaKeys(out.Components.Schemas))
		}
	}
	noUnresolvedPlaceholders(t, out)
}

// ---------------------------------------------------------------------
// body_source
// ---------------------------------------------------------------------

// Locks in the body-source resolver: /create reads r.Body and MUST have
// a requestBody; /sync and /refresh decode from non-request sources
// (an outbound http.Get response and a local file) and MUST NOT.
func TestTestdata_BodySource(t *testing.T) {
	out := loadTestdata(t, "body_source", spec.DefaultHTTPConfig())

	if !hasPath(out, "/create") {
		t.Fatalf("/create missing; have %v", mapPathKeys(out.Paths))
	}
	if firstRequestSchema(t, out, "/create") == nil {
		t.Error("/create should have a requestBody (json.Decode on r.Body)")
	}

	for _, p := range []string{"/sync", "/refresh"} {
		if hasPath(out, p) {
			if firstRequestSchema(t, out, p) != nil {
				t.Errorf("%s should NOT have a requestBody (source is not r.Body)", p)
			}
		}
	}
}

// ---------------------------------------------------------------------
// enum_validation
// ---------------------------------------------------------------------

// Asserts enum/validator-tag features survive into the generated spec:
// at least one component schema must surface an Enum slice somewhere
// in its property tree.
func TestTestdata_EnumValidation(t *testing.T) {
	out := loadTestdata(t, "enum_validation", spec.DefaultHTTPConfig())

	if out.Components == nil || len(out.Components.Schemas) == 0 {
		t.Fatal("expected at least one component schema")
	}

	if !anySchemaHasEnum(out) {
		t.Error("expected at least one property to expose an enum, found none")
	}
	noUnresolvedPlaceholders(t, out)
}

func anySchemaHasEnum(out *spec.OpenAPISpec) bool {
	var walk func(s *intspec.Schema) bool
	walk = func(s *intspec.Schema) bool {
		if s == nil {
			return false
		}
		if len(s.Enum) > 0 {
			return true
		}
		for _, p := range s.Properties {
			if walk(p) {
				return true
			}
		}
		if s.Items != nil && walk(s.Items) {
			return true
		}
		if s.AdditionalProperties != nil && walk(s.AdditionalProperties) {
			return true
		}
		return false
	}
	for _, s := range out.Components.Schemas {
		if walk(s) {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------
// nested_selector
// ---------------------------------------------------------------------

// The fixture wires three handlers through a nested selector
// (`handler.GetServiceHandler`, …). The bug shape this guards against:
// the selector chain failing to resolve and the routes never landing
// in the spec.
func TestTestdata_NestedSelector(t *testing.T) {
	out := loadTestdata(t, "nested_selector", spec.DefaultHTTPConfig())
	for _, p := range []string{"/service", "/handler", "/config"} {
		if !hasPath(out, p) {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
	}
}

// ---------------------------------------------------------------------
// schema
// ---------------------------------------------------------------------

func TestTestdata_Schema(t *testing.T) {
	out := loadTestdata(t, "schema", spec.DefaultHTTPConfig())
	for _, p := range []string{"/user", "/product"} {
		if !hasPath(out, p) {
			t.Errorf("path %q missing; have %v", p, mapPathKeys(out.Paths))
		}
	}
	if out.Components == nil || len(out.Components.Schemas) == 0 {
		t.Error("expected at least one component schema")
	}
	noUnresolvedPlaceholders(t, out)
}

// ---------------------------------------------------------------------
// functional_options (Gorilla Mux + .Methods("GET") fluent chain)
// ---------------------------------------------------------------------

// At least one of the GET-side routes must come through with the right
// method — pins the .Methods("GET") chain resolution.
func TestTestdata_FunctionalOptions(t *testing.T) {
	out := loadTestdata(t, "functional_options", spec.DefaultMuxConfig())

	if !hasPath(out, "/products") {
		t.Fatalf("expected /products; have %v", mapPathKeys(out.Paths))
	}
	if item := out.Paths["/products"]; item.Get == nil {
		t.Errorf("/products should be GET (set via .Methods(\"GET\"))")
	}
}

// ---------------------------------------------------------------------
// dynamic_mount_prefix (chi r.Mount with a computed prefix)
// ---------------------------------------------------------------------

// The fixture mounts the same subrouter under a hard-coded "/v2/api"
// AND under a prefix computed at runtime — that runtime computation is
// the edge case. /v2/api/{id} must always show up; the dynamic mount
// is best-effort.
func TestTestdata_DynamicMountPrefix(t *testing.T) {
	out := loadTestdata(t, "dynamic_mount_prefix", spec.DefaultChiConfig())

	if !hasPath(out, "/v2/api/{id}") {
		t.Errorf("hard-coded mount /v2/api/{id} missing; have %v", mapPathKeys(out.Paths))
	}
}

// ---------------------------------------------------------------------
// router_mount_options
// ---------------------------------------------------------------------

func TestTestdata_RouterMountOptions(t *testing.T) {
	out := loadTestdata(t, "router_mount_options", spec.DefaultChiConfig())
	if len(out.Paths) == 0 {
		t.Fatal("expected at least one path")
	}
	noUnresolvedPlaceholders(t, out)
}

// ---------------------------------------------------------------------
// helper_response_body
// ---------------------------------------------------------------------

// TestTestdata_HelperResponseBody pins the expected behaviour of
// per-route parameter tracing through an indirection helper
// (writeJSON(w, status, v any)). All three handlers feed the helper
// a []items.Item, so all three response schemas MUST resolve to
// `array of $ref(Item)` — the helper's own `v any` parameter must be
// traced back to the caller-site argument for every route, not just
// some.
//
// This test currently asserts the working subset (/a, /b) and
// documents the known regression at /c via a sub-test that runs the
// same assertion. When the underlying bug is fixed — see comment in
// testdata/helper_response_body/main.go — the sub-test will pass on
// its own without changes here, surfacing the fix as a green run.
func TestTestdata_HelperResponseBody(t *testing.T) {
	out := loadTestdata(t, "helper_response_body", spec.DefaultHTTPConfig())

	expectArrayOfItem := func(t *testing.T, path string) {
		t.Helper()
		if !hasPath(out, path) {
			t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
		}
		s := firstResponseSchemaAtStatus(t, out, path, "200")
		if s == nil {
			t.Fatalf("%s 200 response missing schema", path)
		}
		if s.Type != "array" {
			t.Errorf("%s 200 schema should be array, got type=%q "+
				"(the helper's `v any` was not traced back to the caller's typed argument)",
				path, s.Type)
			return
		}
		if s.Items == nil || s.Items.Ref == "" || !strings.HasSuffix(s.Items.Ref, "Item") {
			t.Errorf("%s 200 schema should be array of $ref(Item), got items=%+v", path, s.Items)
		}
	}

	// All three call sites of the shared writeJSON helper must
	// independently resolve to array<$ref(Item)> — i.e. the per-route
	// trace from `v any` back to the caller's typed `out` survives the
	// fact that the underlying Encode call is shared across handlers.
	t.Run("a", func(t *testing.T) { expectArrayOfItem(t, "/a") })
	t.Run("b", func(t *testing.T) { expectArrayOfItem(t, "/b") })
	t.Run("c", func(t *testing.T) { expectArrayOfItem(t, "/c") })

	if componentByName(out, "Item") == nil {
		t.Errorf("Item component missing; have %v", mapSchemaKeys(out.Components.Schemas))
	}
}

// firstResponseSchemaAtStatus returns the response schema attached to a
// specific status code on the path's first operation. Helper local to
// the helper_response_body test because every other test inspects
// either request bodies or the *first* response only.
func firstResponseSchemaAtStatus(t *testing.T, out *spec.OpenAPISpec, path, status string) *intspec.Schema {
	t.Helper()
	item, ok := out.Paths[path]
	if !ok {
		t.Fatalf("path %q missing; have %v", path, mapPathKeys(out.Paths))
	}
	op := firstOperation(&item)
	if op == nil {
		t.Fatalf("no operation on %q", path)
	}
	resp, ok := op.Responses[status]
	if !ok {
		return nil
	}
	for _, media := range resp.Content {
		if media.Schema != nil {
			return media.Schema
		}
	}
	return nil
}

// ---------------------------------------------------------------------
// complex_chi_router  /  another_chi_router
// ---------------------------------------------------------------------

// Broad smoke test: both fixtures exercise many chi features
// (subrouters, render package, middleware, validator tags). Just
// confirm we get paths AND components without leaking placeholders.

func TestTestdata_ComplexChiRouter(t *testing.T) {
	out := loadTestdata(t, "complex_chi_router", spec.DefaultChiConfig())
	if len(out.Paths) < 5 {
		t.Errorf("expected several paths, got %d: %v", len(out.Paths), mapPathKeys(out.Paths))
	}
	if out.Components == nil || len(out.Components.Schemas) == 0 {
		t.Error("expected component schemas")
	}
	noUnresolvedPlaceholders(t, out)
}

func TestTestdata_AnotherChiRouter(t *testing.T) {
	out := loadTestdata(t, "another_chi_router", spec.DefaultChiConfig())
	if len(out.Paths) == 0 {
		t.Fatal("expected at least one path")
	}
	noUnresolvedPlaceholders(t, out)
}
