package spec

import "testing"

func TestIsSubsequence(t *testing.T) {
	cases := []struct {
		a, b []string
		want bool
	}{
		{nil, []string{"api", "user"}, true},                     // empty is a subsequence of anything
		{[]string{"api"}, []string{"api", "user"}, true},         // prefix
		{[]string{"user"}, []string{"api", "user"}, true},        // suffix
		{[]string{"api", "user"}, []string{"api", "user"}, true}, // equal
		{[]string{"v2", "api"}, []string{"mountPoint"}, false},   // distinct mounts
		{[]string{"mountPoint"}, []string{"v2", "api"}, false},
		{[]string{"user", "api"}, []string{"api", "user"}, false}, // order matters
	}
	for _, c := range cases {
		if got := isSubsequence(c.a, c.b); got != c.want {
			t.Errorf("isSubsequence(%v,%v)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestMountSegments(t *testing.T) {
	cases := map[string][]string{
		"":           nil,
		"/":          nil,
		"/api/":      {"api"},
		"/api/user/": {"api", "user"},
		"api/user":   {"api", "user"},
		"/{mount}/":  {"{mount}"},
		"/v2/api":    {"v2", "api"},
	}
	for in, want := range cases {
		got := mountSegments(in)
		if len(got) != len(want) {
			t.Errorf("mountSegments(%q)=%v want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("mountSegments(%q)=%v want %v", in, got, want)
				break
			}
		}
	}
}

func TestDropSubsumedMountPrefixes(t *testing.T) {
	mk := func(fn, method, path, mount string) *RouteInfo {
		return &RouteInfo{Function: fn, Method: method, Path: path, MountPath: mount}
	}

	t.Run("nested mount keeps only the most-mounted", func(t *testing.T) {
		// user.Handler.delete reached at every subset of [api,user].
		routes := []*RouteInfo{
			mk("user.delete", "DELETE", "/{id}", ""),
			mk("user.delete", "DELETE", "/{id}", "/api/"),
			mk("user.delete", "DELETE", "/{id}", "/user/"),
			mk("user.delete", "DELETE", "/{id}", "/api/user/"),
		}
		got := dropSubsumedMountPrefixes(routes)
		if len(got) != 1 || got[0].MountPath != "/api/user/" {
			t.Fatalf("want only /api/user/, got %+v", mountsOf(got))
		}
	})

	t.Run("distinct multi-mounts both survive", func(t *testing.T) {
		// Same sub-router mounted at two genuinely different prefixes.
		routes := []*RouteInfo{
			mk("api.list", "GET", "/{id}", "/v2/api/"),
			mk("api.list", "GET", "/{id}", "/{mountPoint}/"),
		}
		got := dropSubsumedMountPrefixes(routes)
		if len(got) != 2 {
			t.Fatalf("both distinct mounts should survive, got %+v", mountsOf(got))
		}
	})

	t.Run("different handlers at same path are untouched", func(t *testing.T) {
		routes := []*RouteInfo{
			mk("main.FuncLit", "GET", "/", ""),
			mk("user.list", "GET", "/", "/api/user/"),
		}
		got := dropSubsumedMountPrefixes(routes)
		if len(got) != 2 {
			t.Fatalf("distinct functions must not be deduped, got %d", len(got))
		}
	})
}

func mountsOf(rs []*RouteInfo) []string {
	out := make([]string, len(rs))
	for i, r := range rs {
		out[i] = r.MountPath
	}
	return out
}
