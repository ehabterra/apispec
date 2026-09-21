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

// TestMatchesCall pins the accessor matcher, including the case that decides
// whether the whole classifier is safe: an accessor constraining NOTHING must
// match nothing, because it would otherwise make every call a credential read
// and report every middleware as auth — the bug inverted.
func TestMatchesCall(t *testing.T) {
	cases := []struct {
		name            string
		acc             CredentialAccessor
		call, pkg, recv string
		want            bool
	}{
		{
			name: "call, package and receiver all match",
			acc:  CredentialAccessor{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			call: "BasicAuth", pkg: "net/http", recv: "*Request", want: true,
		},
		{
			name: "the qualified receiver spelling",
			// Metadata renders a receiver either way depending on the call;
			// pinning one made r.BasicAuth() invisible.
			acc:  CredentialAccessor{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			call: "BasicAuth", pkg: "net/http", recv: "*net/http.Request", want: true,
		},
		{
			name: "right name, wrong package",
			acc:  CredentialAccessor{CallRegex: `^BasicAuth$`, PkgRegex: `^net/http$`},
			call: "BasicAuth", pkg: "example.com/x", recv: "", want: false,
		},
		{
			name: "right name, wrong receiver",
			acc:  CredentialAccessor{CallRegex: `^Cookie$`, RecvTypeRegex: `^\*?(net/http\.)?Request$`},
			call: "Cookie", pkg: "net/http", recv: "*Response", want: false,
		},
		{
			name: "an accessor that constrains nothing matches nothing",
			acc:  CredentialAccessor{},
			call: "Anything", pkg: "any/pkg", recv: "*Any", want: false,
		},
		{
			name: "an unparseable regex does not match",
			acc:  CredentialAccessor{CallRegex: `^(unclosed`},
			call: "anything", want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := matchesCall(tc.acc, tc.call, tc.pkg, tc.recv); got != tc.want {
				t.Errorf("matchesCall = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCredentialReadConfigEmpty pins the switch that turns the classifier off:
// with no credential surface configured there is nothing to judge by, so the
// classifier must not answer "reads none" for everything — the caller treats
// an empty config as "cannot classify".
func TestCredentialReadConfigEmpty(t *testing.T) {
	if !(CredentialReadConfig{}).empty() {
		t.Error("a zero CredentialReadConfig is not reported empty")
	}
	if (CredentialReadConfig{NameRegexes: []string{"x"}}).empty() {
		t.Error("a config with names is reported empty")
	}
	if (CredentialReadConfig{Accessors: []CredentialAccessor{{CallRegex: "x"}}}).empty() {
		t.Error("a config with accessors is reported empty")
	}
}

// TestStdlibCredentialNamesAreWholeNames guards the one thing a name table must
// not do: match a header that merely looks credential-adjacent. X-Request-Id
// and X-Api-Version are on every service, and matching either would report its
// logging middleware as auth — the noise this issue is about.
func TestStdlibCredentialNamesAreWholeNames(t *testing.T) {
	cred := stdlibCredentialReads()
	match := func(s string) bool {
		for _, re := range cred.NameRegexes {
			if compiled, err := cachedRegex(re); err == nil && compiled.MatchString(s) {
				return true
			}
		}
		return false
	}

	for _, credential := range []string{
		"Authorization", "authorization", "Proxy-Authorization",
		"X-Api-Key", "x-api-key", "X-API-KEY", "X-Auth-Token", "X-Access-Token",
		"Api-Key", "apikey", "auth_token",
	} {
		if !match(credential) {
			t.Errorf("%q is a credential name and was not recognised", credential)
		}
	}

	for _, ordinary := range []string{
		"X-Request-Id", "X-Api-Version", "Content-Type", "Accept",
		"X-Forwarded-For", "User-Agent", "X-Trace-Id", "If-None-Match",
		// Close enough to matter: these are not credentials.
		"X-Api-Deprecation", "Authorization-Info", "X-Keyboard",
	} {
		if match(ordinary) {
			t.Errorf("%q is an ordinary header and was read as a credential", ordinary)
		}
	}
}

// TestStatusArgValue pins reading a status out of an argument in the spellings
// a handler writes one. A status that does not resolve is not a refusal, so a
// miss here costs a silent unreported auth middleware.
func TestStatusArgValue(t *testing.T) {
	meta := newTestMeta()

	t.Run("a bare number", func(t *testing.T) {
		arg := metadata.NewCallArgument(meta)
		arg.SetKind(metadata.KindLiteral)
		arg.SetValue("401")
		if code, ok := statusArgValue(arg); !ok || code != 401 {
			t.Errorf("statusArgValue = (%d, %v), want (401, true)", code, ok)
		}
	})

	t.Run("net/http's constant", func(t *testing.T) {
		// `http.StatusForbidden` — matched on the trailing identifier, so an
		// aliased import resolves the same way.
		arg := mkSelector(meta, mkIdent(meta, "http", ""), mkIdent(meta, "StatusForbidden", ""))
		if code, ok := statusArgValue(arg); !ok || code != 403 {
			t.Errorf("statusArgValue = (%d, %v), want (403, true)", code, ok)
		}
	})

	t.Run("a dot-imported constant", func(t *testing.T) {
		if code, ok := statusArgValue(mkIdent(meta, "StatusUnauthorized", "")); !ok || code != 401 {
			t.Errorf("statusArgValue = (%d, %v), want (401, true)", code, ok)
		}
	})

	t.Run("not a status", func(t *testing.T) {
		lit := metadata.NewCallArgument(meta)
		lit.SetKind(metadata.KindLiteral)
		lit.SetValue(`"unauthorized"`)
		if _, ok := statusArgValue(lit); ok {
			t.Error("a message string was read as a status")
		}
		if _, ok := statusArgValue(mkIdent(meta, "someLocal", "")); ok {
			t.Error("an unrelated identifier was read as a status")
		}
	})
}

// TestRefusalStatusesAreAuthOnly guards the status table against the failure
// that would undo the fix: a refusing middleware is not an authenticating one.
// A rate limiter (429), a size limiter (413) and a timeout (504) all refuse.
func TestRefusalStatusesAreAuthOnly(t *testing.T) {
	cred := stdlibCredentialReads()

	if !cred.refuses(401) {
		t.Error("401 means the request was not authenticated and is not counted")
	}
	// 403 is authorisation: a role gate behind the real auth middleware
	// refuses with it and reads no credential. Counting it reported those
	// gates as unmapped authentication.
	for _, refusal := range []int{400, 403, 404, 409, 413, 415, 422, 429, 500, 503, 504} {
		if cred.refuses(refusal) {
			t.Errorf("%d is a refusal but not an auth one; counting it reports every guard as auth", refusal)
		}
	}
}

// TestCallNamesCredentialNeedsARead pins that a credential NAME counts only as
// the argument of a by-name read.
//
// A literal on its own is not evidence about what a middleware does — the same
// string appears in a log line, and in a proxy middleware SETTING an outbound
// Authorization header, which is the opposite of reading one.
func TestCallNamesCredentialNeedsARead(t *testing.T) {
	meta := newTestMeta()
	e := &Extractor{cfg: &APISpecConfig{}, contextProvider: NewContextProvider(meta)}
	cred := stdlibCredentialReads()

	lit := func(v string) *metadata.CallArgument {
		a := metadata.NewCallArgument(meta)
		a.SetKind(metadata.KindLiteral)
		a.SetValue(`"` + v + `"`)
		return a
	}
	edge := func(args ...*metadata.CallArgument) *metadata.CallGraphEdge {
		return &metadata.CallGraphEdge{Args: args}
	}

	cases := []struct {
		name            string
		call, pkg, recv string
		args            []*metadata.CallArgument
		want            bool
		why             string
	}{
		{
			name: "a header read naming a credential",
			call: "Get", pkg: "net/http", recv: "Header", args: []*metadata.CallArgument{lit("Authorization")},
			want: true, why: "this is the shape the signal is about",
		},
		{
			name: "a header read naming an ordinary header",
			call: "Get", pkg: "net/http", recv: "Header", args: []*metadata.CallArgument{lit("X-Request-Id")},
			want: false, why: "every service reads this one",
		},
		{
			name: "the word logged, not read",
			call: "Println", pkg: "log", recv: "", args: []*metadata.CallArgument{lit("Authorization")},
			want: false, why: "a literal is not evidence about what the call does",
		},
		{
			name: "an outbound header being SET",
			call: "Set", pkg: "net/http", recv: "Header", args: []*metadata.CallArgument{lit("Authorization"), lit("Bearer x")},
			want: false, why: "setting a credential is the opposite of reading one",
		},
		{
			name: "a read with no credential in it",
			call: "Get", pkg: "net/http", recv: "Header", args: nil,
			want: false, why: "a read alone says nothing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := e.callNamesCredential(edge(tc.args...), cred, tc.call, tc.pkg, tc.recv)
			if got != tc.want {
				t.Errorf("callNamesCredential = %v, want %v — %s", got, tc.want, tc.why)
			}
		})
	}

	t.Run("no names configured", func(t *testing.T) {
		if e.callNamesCredential(edge(lit("Authorization")), CredentialReadConfig{}, "Get", "net/http", "Header") {
			t.Error("matched with an empty configuration")
		}
	})
}

// TestCallRefusesWithAuthStatusNeedsAWriter pins that a 401/403 counts only
// where a status is actually WRITTEN — which the framework's own response
// patterns already declare, so this stays framework-agnostic.
func TestCallRefusesWithAuthStatusNeedsAWriter(t *testing.T) {
	meta := newTestMeta()
	cfg := DefaultChiConfig()
	e := &Extractor{cfg: cfg, contextProvider: NewContextProvider(meta)}
	cred := stdlibCredentialReads()

	status := func(name string) *metadata.CallArgument {
		return mkSelector(meta, mkIdent(meta, "http", ""), mkIdent(meta, name, ""))
	}
	msg := metadata.NewCallArgument(meta)
	msg.SetKind(metadata.KindLiteral)
	msg.SetValue(`"forbidden"`)
	w := mkIdent(meta, "w", "net/http.ResponseWriter")

	sp := meta.StringPool
	call := func(name, pkg, recv string, args ...*metadata.CallArgument) *metadata.CallGraphEdge {
		return &metadata.CallGraphEdge{
			Callee: metadata.Call{Meta: meta, Name: sp.Get(name), Pkg: sp.Get(pkg), RecvType: sp.Get(recv)},
			Args:   args,
		}
	}

	t.Run("http.Error with 401", func(t *testing.T) {
		// A package-level writer: the response pattern scopes it by PACKAGE in
		// RecvTypeRegex, and the call records no receiver. Reading the field
		// literally matched nothing and lost every http.Error refusal.
		if !e.callRefusesWithAuthStatus(call("Error", "net/http", "", w, msg, status("StatusUnauthorized")), cred) {
			t.Error("http.Error(w, msg, 401) is a refusal and was not recognised")
		}
	})

	t.Run("http.Error with 403 is authorisation", func(t *testing.T) {
		if e.callRefusesWithAuthStatus(call("Error", "net/http", "", w, msg, status("StatusForbidden")), cred) {
			t.Error("403 is an authorisation refusal and must not signal authentication")
		}
	})

	t.Run("a non-auth refusal", func(t *testing.T) {
		if e.callRefusesWithAuthStatus(call("Error", "net/http", "", w, msg, status("StatusTooManyRequests")), cred) {
			t.Error("429 is a refusal but not an auth one")
		}
	})

	t.Run("403 passed to something that writes nothing", func(t *testing.T) {
		if e.callRefusesWithAuthStatus(call("Observe", "example.com/metrics", "", status("StatusForbidden")), cred) {
			t.Error("a metric taking 403 was read as a refusal")
		}
	})

	t.Run("no refusal statuses configured", func(t *testing.T) {
		if e.callRefusesWithAuthStatus(call("Error", "net/http", "", w, msg, status("StatusUnauthorized")),
			CredentialReadConfig{}) {
			t.Error("matched with an empty configuration")
		}
	})
}
