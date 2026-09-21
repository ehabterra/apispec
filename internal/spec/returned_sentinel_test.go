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

const (
	echoPkg       = "github.com/labstack/echo/v4"
	echoErrorType = "*github.com/labstack/echo/v4.HTTPError"
)

func echoSentinel() ErrorSentinel {
	return DefaultEchoConfig().Framework.ErrorSentinels[0]
}

// The status comes from the sentinel's NAME, and only when every scope matcher
// agrees: a same-named value of another package or type is not the framework's.
func TestSentinelStatus(t *testing.T) {
	s := echoSentinel()
	for _, tc := range []struct {
		name, pkg, typ string
		want           int
	}{
		{"ErrForbidden", echoPkg, echoErrorType, 403},
		{"ErrNotFound", echoPkg, echoErrorType, 404},
		// echo spells this one with the table's own prefix.
		{"ErrStatusRequestEntityTooLarge", echoPkg, echoErrorType, 413},
		// A major version without a /vN suffix is still echo.
		{"ErrForbidden", "github.com/labstack/echo", "*github.com/labstack/echo.HTTPError", 403},
	} {
		if got, ok := sentinelStatus(s, tc.name, tc.pkg, tc.typ); !ok || got != tc.want {
			t.Errorf("%s: got %d, %v; want %d", tc.name, got, ok, tc.want)
		}
	}

	for _, tc := range []struct{ why, name, pkg, typ string }{
		{"a name naming no status", "ErrValidatorNotRegistered", echoPkg, echoErrorType},
		{"a plain error, not an *HTTPError", "ErrForbidden", echoPkg, "error"},
		{"another package's same-named value", "ErrForbidden", "example.com/app", echoErrorType},
		{"no Err prefix", "Forbidden", echoPkg, echoErrorType},
		{"an empty capture", "Err", echoPkg, echoErrorType},
	} {
		if got, ok := sentinelStatus(s, tc.name, tc.pkg, tc.typ); ok {
			t.Errorf("%s: resolved to %d, want no status", tc.why, got)
		}
	}

	// A sentinel with no package scope claims nothing: it would otherwise match
	// every Err* value in the program.
	unscoped := s
	unscoped.PkgRegex = ""
	if _, ok := sentinelStatus(unscoped, "ErrForbidden", echoPkg, echoErrorType); ok {
		t.Error("an unscoped sentinel resolved a status")
	}
	broken := s
	broken.NameRegex = "("
	if _, ok := sentinelStatus(broken, "ErrForbidden", echoPkg, echoErrorType); ok {
		t.Error("an invalid name regex resolved a status")
	}
}

// sentinelSelector builds `echo.ErrForbidden` as the metadata records a
// returned package variable.
func sentinelSelector(meta *metadata.Metadata, name string) *metadata.CallArgument {
	sel := mkIdentPkg(meta, name, echoPkg)
	sel.Type = meta.StringPool.Get(echoErrorType)
	return mkSelector(meta, mkIdent(meta, "echo", ""), sel)
}

func sentinelRoute(meta *metadata.Metadata, returns ...*metadata.CallArgument) *RouteInfo {
	fn := &metadata.Function{Name: meta.StringPool.Get("get"), Pkg: meta.StringPool.Get("app")}
	for _, r := range returns {
		fn.Returns = append(fn.Returns, []metadata.CallArgument{*r})
	}
	meta.Packages = map[string]*metadata.Package{"app": {Files: map[string]*metadata.File{
		"main.go": {Functions: map[string]*metadata.Function{"get": fn}},
	}}}
	return &RouteInfo{
		Function: "get", Package: "app", Metadata: meta,
		UsedTypes: map[string]*Schema{},
		Response:  map[string]*ResponseInfo{"200": {StatusCode: 200}},
	}
}

func TestAddReturnedSentinels(t *testing.T) {
	cfg := DefaultEchoConfig()

	t.Run("each returned sentinel documents its status", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		route := sentinelRoute(meta,
			sentinelSelector(meta, "ErrForbidden"),
			sentinelSelector(meta, "ErrNotFound"),
			mkIdent(meta, "nil", ""))
		(&Extractor{cfg: cfg}).addReturnedSentinels(route)

		for _, status := range []string{"403", "404"} {
			r := route.Response[status]
			if r == nil {
				t.Fatalf("no %s; have %v", status, route.Response)
			}
			if r.ContentType != cfg.Defaults.ResponseContentType {
				t.Errorf("%s content type = %q, want the default", status, r.ContentType)
			}
			// BodyFromValue + Deref: the body is the value's pointee.
			if r.BodyType != "github.com/labstack/echo/v4.HTTPError" {
				t.Errorf("%s body type = %q, want the dereferenced *HTTPError", status, r.BodyType)
			}
		}
		if len(route.Response) != 3 {
			t.Errorf("responses = %v, want 200 plus the two sentinels", route.Response)
		}
	})

	// A status the route already documents keeps its description: a
	// constructor or a context write is the more specific statement.
	t.Run("a documented status is not replaced", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		route := sentinelRoute(meta, sentinelSelector(meta, "ErrForbidden"))
		written := &ResponseInfo{StatusCode: 403, BodyType: "app.Denied"}
		route.Response["403"] = written
		(&Extractor{cfg: cfg}).addReturnedSentinels(route)
		if route.Response["403"] != written {
			t.Errorf("403 = %+v, want the one the handler writes", route.Response["403"])
		}
	})

	// A body-typed sentinel (fiber writes the message as text) takes its
	// declared type and media type rather than the value's.
	t.Run("a declared body type", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		route := sentinelRoute(meta, sentinelSelector(meta, "ErrForbidden"))
		c := DefaultEchoConfig()
		c.Framework.ErrorSentinels = []ErrorSentinel{{
			PkgRegex: `^github\.com/labstack/echo`, NameRegex: `^Err(\w+)$`,
			BodyType: "string", ContentType: "text/plain",
		}}
		(&Extractor{cfg: c}).addReturnedSentinels(route)
		r := route.Response["403"]
		if r == nil || r.ContentType != "text/plain" || r.Schema == nil || r.Schema.Type != "string" {
			t.Errorf("403 = %+v, want a text/plain string", r)
		}
	})

	t.Run("nothing configured or nothing resolvable", func(t *testing.T) {
		meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
		route := sentinelRoute(meta, sentinelSelector(meta, "ErrForbidden"))
		(&Extractor{cfg: DefaultHTTPConfig()}).addReturnedSentinels(route)
		(&Extractor{}).addReturnedSentinels(route)
		(&Extractor{cfg: cfg}).addReturnedSentinels(&RouteInfo{})
		if len(route.Response) != 1 {
			t.Errorf("responses = %v, want only the 200", route.Response)
		}
	})
}

// Sentinels are package-scoped, so both merge paths carry them: a secondary
// framework's sentinel can only ever claim its own framework's values.
func TestErrorSentinelsMerge(t *testing.T) {
	if got := SecondaryView(DefaultFiberConfig()).Framework.ErrorSentinels; len(got) != 1 {
		t.Errorf("SecondaryView dropped fiber's sentinels: %v", got)
	}
	merged := MergeFrameworkConfigs(DefaultEchoConfig(), DefaultFiberConfig(), DefaultEchoConfig())
	if got := merged.Framework.ErrorSentinels; len(got) != 2 {
		t.Errorf("merged sentinels = %v, want echo's and fiber's once each", got)
	}
}
