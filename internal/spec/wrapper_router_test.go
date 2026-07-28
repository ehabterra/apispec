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
	"reflect"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// verbArg builds the argument a registrar receives its verb in.
func verbArg(m *metadata.Metadata, value string) *metadata.CallArgument {
	arg := metadata.NewCallArgument(m)
	arg.SetKind(metadata.KindLiteral)
	arg.SetValue(`"` + value + `"`)
	return arg
}

// TestVerbsFromArg covers the verb-carrying argument a house router registers
// through (issue #221). A list is the interesting case — gitea's
// `Methods("GET,POST", …)` registers both — and so is the refusal: a value that is
// only partly verb-like means the argument was not understood, and inventing a
// route from half of it is worse than leaving the default.
func TestVerbsFromArg(t *testing.T) {
	m := wireNameMeta()
	matcher := NewRoutePatternMatcher(
		RoutePattern{CallRegex: "^Methods$", MethodFromArg: true},
		&APISpecConfig{}, NewContextProvider(m), nil,
	)

	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"one verb", "GET", []string{"GET"}},
		{"lower case", "get", []string{"GET"}},
		{"two verbs", "GET,POST", []string{"GET", "POST"}},
		{"spaces around the separator", "GET, POST , PUT", []string{"GET", "POST", "PUT"}},
		{"a repeat is listed once", "GET,GET,POST", []string{"GET", "POST"}},
		{"every verb has to be one", "GET,everything", nil},
		{"not a verb at all", "/users", nil},
		{"empty", "", nil},
		{"a trailing separator names nothing", "GET,", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := matcher.verbsFromArg(verbArg(m, tc.value))
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("verbsFromArg(%q) = %v, want %v", tc.value, got, tc.want)
			}
		})
	}

	if got := matcher.verbsFromArg(nil); got != nil {
		t.Errorf("nil argument = %v, want nil", got)
	}
	// A value that is not statically known names no verb: a pattern relying on
	// `http.MethodGet` (a package apispec does not analyse) keeps the older
	// single-verb fallback instead of guessing here.
	unknown := metadata.NewCallArgument(m)
	unknown.SetKind(metadata.KindIdent)
	unknown.SetName("verb")
	if got := matcher.verbsFromArg(unknown); got != nil {
		t.Errorf("unresolvable argument = %v, want nil", got)
	}
}

// TestExpandMultiVerbRoutes pins what one registration naming several verbs
// becomes: one route per verb, each with its own operationId, sharing everything
// the handler determines.
func TestExpandMultiVerbRoutes(t *testing.T) {
	plain := &RouteInfo{Method: "GET", Path: "/items", Function: "listItems"}
	multi := &RouteInfo{
		Method:       "GET",
		ExtraMethods: []string{"POST", "PUT"},
		Path:         "/search",
		Function:     "searchItems",
		Request:      &RequestInfo{},
	}

	got := expandMultiVerbRoutes([]*RouteInfo{plain, multi})
	if len(got) != 4 {
		t.Fatalf("expanded to %d routes, want 4 (1 + 3)", len(got))
	}
	if got[0] != plain {
		t.Error("a single-verb route should pass through untouched")
	}
	if got[0].OperationIDSuffix != "" {
		t.Errorf("single-verb suffix = %q, want none", got[0].OperationIDSuffix)
	}

	seen := map[string]string{}
	for _, r := range got[1:] {
		seen[r.Method] = r.OperationIDSuffix
		if r.Path != "/search" || r.Function != "searchItems" {
			t.Errorf("%s: path/handler changed (%s %s)", r.Method, r.Path, r.Function)
		}
		if r.Request == nil {
			t.Errorf("%s: the verbs of one registration share the handler's request", r.Method)
		}
		if len(r.ExtraMethods) != 0 {
			t.Errorf("%s: still carries ExtraMethods %v", r.Method, r.ExtraMethods)
		}
	}
	want := map[string]string{"GET": "GET", "POST": "POST", "PUT": "PUT"}
	if !reflect.DeepEqual(seen, want) {
		t.Errorf("methods/suffixes = %v, want %v — every verb needs its own operationId", seen, want)
	}
}

// TestHandlerArgValue covers looking through a type conversion to the handler.
// `http.HandlerFunc(fn)` names a type, and a type has no doc comment, no request
// and no responses — which is what the operation lost.
func TestHandlerArgValue(t *testing.T) {
	m := wireNameMeta()
	conversion := func(inner *metadata.CallArgument) *metadata.CallArgument {
		conv := metadata.NewCallArgument(m)
		conv.SetKind(metadata.KindTypeConversion)
		fun := metadata.NewCallArgument(m)
		fun.SetKind(metadata.KindSelector)
		conv.Fun = fun
		if inner != nil {
			conv.Args = []*metadata.CallArgument{inner}
		}
		return conv
	}
	of := func(kind, name string) *metadata.CallArgument {
		a := metadata.NewCallArgument(m)
		a.SetKind(kind)
		a.SetName(name)
		return a
	}

	// Peeled: each of these can BE a handler.
	for _, kind := range []string{metadata.KindIdent, metadata.KindSelector, metadata.KindFuncLit} {
		inner := of(kind, "listItems")
		if got := handlerArgValue(conversion(inner)); got != inner {
			t.Errorf("conversion of a %s was not looked through", kind)
		}
	}

	// Left alone: a conversion of something that cannot be a handler, where the
	// conversion's own type is usually the answer (`[]byte(body)`).
	literal := metadata.NewCallArgument(m)
	literal.SetKind(metadata.KindLiteral)
	literal.SetValue(`"x"`)
	conv := conversion(literal)
	if got := handlerArgValue(conv); got != conv {
		t.Error("a conversion of a literal should be left as it is")
	}
	// A conversion with no argument, and a plain argument.
	empty := conversion(nil)
	if got := handlerArgValue(empty); got != empty {
		t.Error("an argument-less conversion should be left as it is")
	}
	plain := of(metadata.KindIdent, "listItems")
	if got := handlerArgValue(plain); got != plain {
		t.Error("a plain handler argument should be returned unchanged")
	}
	if got := handlerArgValue(nil); got != nil {
		t.Error("nil should stay nil")
	}
}

// TestCompletesSameRegistration pins the rule that keeps a framework's own
// patterns from re-matching inside a wrapper: a child may finish the route its
// parent started only when it belongs to the same registration.
func TestCompletesSameRegistration(t *testing.T) {
	m := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	e := &Extractor{}

	// A call written in caller, at position pos.
	callNode := func(caller, pos string) *TrackerNode {
		return &TrackerNode{CallGraphEdge: &metadata.CallGraphEdge{
			Caller: metadata.Call{Name: m.StringPool.Get(caller), Pkg: m.StringPool.Get("app"), Position: m.StringPool.Get(pos), Meta: m},
			Callee: metadata.Call{Name: m.StringPool.Get("Get"), Pkg: m.StringPool.Get("app"), Meta: m},
		}}
	}

	route := callNode("main", "main.go:10:2")

	t.Run("a call in the same function may complete it", func(t *testing.T) {
		// A fluent chain: Methods("GET").Path("/x") — written where the route is.
		if !e.completesSameRegistration(route, callNode("main", "main.go:10:20")) {
			t.Error("a sibling call in the same function was rejected")
		}
	})

	t.Run("a call inside the callee may not", func(t *testing.T) {
		// The wrapper's delegate: chi.Mux.Method(...) inside Router.Methods.
		if e.completesSameRegistration(route, callNode("Methods", "router.go:46:2")) {
			t.Error("a call inside another function was allowed to overwrite the route")
		}
	})

	t.Run("an explicit chain link decides on its own", func(t *testing.T) {
		chained := callNode("other", "router.go:5:2")
		chained.ChainParent = route.CallGraphEdge
		if !e.completesSameRegistration(route, chained) {
			t.Error("a chain continuation was rejected")
		}
	})

	t.Run("an argument is not a registration", func(t *testing.T) {
		arg := callNode("main", "main.go:10:2")
		arg.CallArgument = metadata.NewCallArgument(m)
		if e.completesSameRegistration(route, arg) {
			t.Error("an argument node was treated as a registration — its call would be re-extracted from the argument")
		}
	})

	t.Run("nothing to compare", func(t *testing.T) {
		if e.completesSameRegistration(nil, route) || e.completesSameRegistration(route, nil) {
			t.Error("a missing node was accepted")
		}
		if e.completesSameRegistration(route, &TrackerNode{}) {
			t.Error("a node with no edge was accepted")
		}
	})
}
