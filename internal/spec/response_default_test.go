package spec

import "testing"

func TestBuildResponses_UninformativeDefaultDropped(t *testing.T) {
	ref := func(name string) *Schema { return &Schema{Ref: "#/components/schemas/" + name} }

	t.Run("redundant default (same body as resolved) is dropped", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"201": {StatusCode: 201, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
		})
		if _, ok := r["default"]; ok {
			t.Error("default with a body already at a resolved status should be dropped")
		}
		if _, ok := r["201"]; !ok {
			t.Error("the resolved 201 must remain")
		}
	})

	t.Run("generic-object default is dropped when resolved statuses exist", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"200": {StatusCode: 200, ContentType: "application/json", BodyType: "[]Item", Schema: &Schema{Type: "array", Items: ref("Item")}},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "any", Schema: &Schema{Type: "object"}},
		})
		if _, ok := r["default"]; ok {
			t.Error("bare generic-object default should be dropped alongside resolved statuses")
		}
	})

	t.Run("distinct concrete default is kept", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"400": {StatusCode: 400, ContentType: "application/json", BodyType: "ErrorResponse", Schema: ref("ErrorResponse")},
			"-1":  {StatusCode: -1, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
		})
		if _, ok := r["default"]; !ok {
			t.Error("a default carrying a distinct concrete body must be kept")
		}
	})

	t.Run("default kept when it is the only response", func(t *testing.T) {
		r := buildResponses(map[string]*ResponseInfo{
			"-1": {StatusCode: -1, ContentType: "application/json", BodyType: "User", Schema: ref("User")},
		})
		if _, ok := r["default"]; !ok {
			t.Error("a sole default must be kept")
		}
	})
}
