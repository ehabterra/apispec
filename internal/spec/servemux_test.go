package spec

import "testing"

func TestSplitMethodFromPath(t *testing.T) {
	cases := []struct {
		in         string
		wantMethod string
		wantPath   string
	}{
		// Go 1.22 ServeMux method-aware patterns.
		{`"GET /users/{id}"`, "GET", "/users/{id}"},
		{`"POST /users"`, "POST", "/users"},
		{`"get /health"`, "GET", "/health"}, // verb is upper-cased
		{`"DELETE /users/{id}"`, "DELETE", "/users/{id}"},
		// Plain net/http patterns pass through unchanged.
		{`"/health"`, "", "/health"},
		{`"/users/{id}"`, "", "/users/{id}"},
		// A leading non-verb token is not treated as a method.
		{`"FOO /bar"`, "", "FOO /bar"},
		{"", "", ""},
	}
	for _, c := range cases {
		gotMethod, gotPath := splitMethodFromPath(c.in)
		if gotMethod != c.wantMethod || gotPath != c.wantPath {
			t.Errorf("splitMethodFromPath(%q) = (%q, %q); want (%q, %q)",
				c.in, gotMethod, gotPath, c.wantMethod, c.wantPath)
		}
	}
}

func TestNormalizeServeMuxPath(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/users/{id}", "/users/{id}"},
		{"/files/{path...}", "/files/{path}"}, // trailing wildcard
		{"/items/{$}", "/items/"},             // end-of-path anchor dropped
		{"/static/{dir...}/{$}", "/static/{dir}/"},
	}
	for _, c := range cases {
		if got := normalizeServeMuxPath(c.in); got != c.want {
			t.Errorf("normalizeServeMuxPath(%q) = %q; want %q", c.in, got, c.want)
		}
	}
}
