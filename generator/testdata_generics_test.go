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

	for _, p := range []string{"/users", "/products", "/user", "/pair", "/nested", "/inferred", "/create", "/batch"} {
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
	// []byte fields must render the way encoding/json marshals them: a
	// base64 string, not an array of integers.
	if avatar := userSchema.Properties["avatar"]; avatar == nil || avatar.Type != "string" || avatar.Format != "byte" {
		t.Errorf("User.avatar = %+v, want {type: string, format: byte}", avatar)
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

	// Pair[User, Product]: two type parameters must each map to their own
	// concrete argument — First->User, Second->Product — not collapse together.
	_, pair := findSchema("_structs_Pair_User-Product")
	if pair == nil {
		t.Fatalf("Pair_User-Product component missing; have %v", mapSchemaKeys(schemas))
	}
	if first := pair.Properties["first"]; first == nil || !strings.HasSuffix(first.Ref, "_User") {
		t.Errorf("Pair.first = %+v, want a $ref to the User component", first)
	}
	if second := pair.Properties["second"]; second == nil || !strings.HasSuffix(second.Ref, "_Product") {
		t.Errorf("Pair.second = %+v, want a $ref to the Product component", second)
	}

	// Nested: Envelope[Page[User]] — the type argument is itself a generic
	// instantiation. data must resolve to the Page[User] envelope (not a
	// placeholder), and that Page must carry items -> $ref User.
	_, nested := findSchema("_structs_Envelope_Page_User")
	if nested == nil {
		t.Fatalf("Envelope_Page_User (nested) component missing; have %v", mapSchemaKeys(schemas))
	}
	nestedData := nested.Properties["data"]
	if nestedData == nil || !strings.HasSuffix(nestedData.Ref, "_Page_User") {
		t.Fatalf("nested Envelope.data = %+v, want a $ref to the Page_User component", nestedData)
	}
	_, nestedPage := findSchema("_structs_Page_User")
	if nestedPage == nil || nestedPage.Properties["items"] == nil ||
		nestedPage.Properties["items"].Items == nil ||
		!strings.HasSuffix(nestedPage.Properties["items"].Items.Ref, "_User") {
		t.Errorf("nested Page_User.items = %+v, want array of $ref User", nestedPage)
	}

	// Inferred: NewEnvelope(products[0]) is Envelope[Product] with no explicit
	// [Product] at the encode site. It must resolve to the same clean component
	// as a written Envelope[Product] (data -> $ref Product), not embed the
	// argument's full package path in the name.
	_, inferred := findSchema("_structs_Envelope_Product")
	if inferred == nil {
		t.Fatalf("Envelope_Product (inferred) component missing; have %v", mapSchemaKeys(schemas))
	}
	if data := inferred.Properties["data"]; data == nil || !strings.HasSuffix(data.Ref, "_Product") {
		t.Errorf("inferred Envelope.data = %+v, want a $ref to the Product component", data)
	}

	// Request body: POST /create decodes Page[User]. A generic request body must
	// key to the SAME clean component as the Page[User] response (listUsers), not
	// a separate verbose duplicate.
	createReq := opFor(out.Paths["/create"], "POST")
	if createReq == nil || createReq.RequestBody == nil {
		t.Fatalf("POST /create missing request body")
	}
	var reqRef string
	for _, mt := range createReq.RequestBody.Content {
		if mt.Schema != nil {
			reqRef = mt.Schema.Ref
		}
	}
	if !strings.HasSuffix(reqRef, "_Page_User") {
		t.Errorf("POST /create request body ref = %q, want the shared Page_User component", reqRef)
	}
	// Exactly one Page_User component — request and response share it, no
	// verbose duplicate. Anchored on "_structs_Page_User" so the nested
	// Envelope_Page_User component isn't counted.
	pageUserComponents := 0
	for k := range schemas {
		if strings.HasSuffix(k, "_structs_Page_User") {
			pageUserComponents++
		}
	}
	if pageUserComponents != 1 {
		t.Errorf("want exactly 1 Page_User component (request and response share it), got %d: %v", pageUserComponents, mapSchemaKeys(schemas))
	}

	// Wrapped instantiation: POST /batch encodes []Envelope[User] — a slice of
	// a generic instantiation. The concrete argument must survive the slice
	// constructor: the response is an array whose items $ref the same
	// Envelope_User component as the direct /user route (no Envelope_T-any
	// placeholder resurrected).
	batchOp := opFor(out.Paths["/batch"], "POST")
	if batchOp == nil {
		t.Fatalf("POST /batch missing")
	}
	var batchSchema *intspec.Schema
	for _, resp := range batchOp.Responses {
		for _, mt := range resp.Content {
			if mt.Schema != nil {
				batchSchema = mt.Schema
			}
		}
	}
	if batchSchema == nil || batchSchema.Type != "array" || batchSchema.Items == nil ||
		!strings.HasSuffix(batchSchema.Items.Ref, "_Envelope_User") {
		t.Errorf("POST /batch response = %+v, want array of $ref Envelope_User", batchSchema)
	}

	// No lingering unsubstituted placeholder (the pre-fix Page_T-any / T-any).
	for k := range schemas {
		if strings.HasSuffix(k, "_T-any") || strings.HasSuffix(k, "_T") {
			t.Errorf("found unsubstituted generic placeholder component %q", k)
		}
	}
}
