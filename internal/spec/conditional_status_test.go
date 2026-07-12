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
	"sort"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// TestExtractResponse_BranchedStatuses mirrors issue #39: an error variable
// reassigned across if/else branches with different status codes should
// produce one ResponseInfo per distinct status.
//
//	if errors.As(err, &a401) {
//	    err = NewError(msg, http.StatusUnauthorized)   // 401
//	} else if errors.As(err, &a404) {
//	    err = NewError(msg, http.StatusNotFound)       // 404
//	} else {
//	    err = NewError(msg, http.StatusInternalServerError) // 500
//	}
//	RespondWithError(w, err)
func TestExtractResponse_BranchedStatuses(t *testing.T) {
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Packages: map[string]*metadata.Package{
			"app": {Files: map[string]*metadata.File{}},
		},
	}

	// Build three assignments, each a call to NewError(msg, http.StatusXxx).
	assigns := make([]metadata.Assignment, 0, 3)
	for _, status := range []string{
		"StatusUnauthorized",
		"StatusNotFound",
		"StatusInternalServerError",
	} {
		statusArg := metadata.NewCallArgument(meta)
		statusArg.SetKind(metadata.KindSelector)
		// Render as "http.<Name>" so MapStatusCode's last-dot split picks
		// up the constant name.
		x := metadata.NewCallArgument(meta)
		x.SetKind(metadata.KindIdent)
		x.SetName("http")
		sel := metadata.NewCallArgument(meta)
		sel.SetKind(metadata.KindIdent)
		sel.SetName(status)
		statusArg.X = x
		statusArg.Sel = sel

		msgArg := metadata.NewCallArgument(meta)
		msgArg.SetKind(metadata.KindIdent)
		msgArg.SetName("msg")

		rhs := metadata.NewCallArgument(meta)
		rhs.SetKind(metadata.KindCall)
		fun := metadata.NewCallArgument(meta)
		fun.SetKind(metadata.KindIdent)
		fun.SetName("NewError")
		rhs.Fun = fun
		rhs.Args = []*metadata.CallArgument{msgArg, statusArg}

		assigns = append(assigns, metadata.Assignment{
			Value: *rhs,
		})
	}

	// Register the handler function with the branched err assignments.
	handlerFn := &metadata.Function{
		AssignmentMap: map[string][]metadata.Assignment{
			"err": assigns,
		},
	}
	meta.Packages["app"].Files["handlers.go"] = &metadata.File{
		Functions: map[string]*metadata.Function{
			"handler": handlerFn,
		},
	}

	// Build the call to RespondWithError(w, err): arg0 = w, arg1 = err.
	errArg := metadata.NewCallArgument(meta)
	errArg.SetKind(metadata.KindIdent)
	errArg.SetName("err")
	wArg := metadata.NewCallArgument(meta)
	wArg.SetKind(metadata.KindIdent)
	wArg.SetName("w")

	edge := &metadata.CallGraphEdge{
		Caller: metadata.Call{
			Name: meta.StringPool.Get("handler"),
			Pkg:  meta.StringPool.Get("app"),
		},
		Callee: metadata.Call{
			Name: meta.StringPool.Get("RespondWithError"),
			Pkg:  meta.StringPool.Get("app"),
		},
		Args: []*metadata.CallArgument{wArg, errArg},
	}
	node := &TrackerNode{CallGraphEdge: edge}

	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	cp := NewContextProvider(meta)
	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			cfg:             cfg,
			contextProvider: cp,
			schemaMapper:    NewSchemaMapper(cfg),
		},
		pattern: ResponsePattern{
			CallRegex:      "^RespondWithError$",
			StatusFromArg:  true,
			StatusArgIndex: 1,
		},
	}

	results := matcher.ExtractResponse(node, &RouteInfo{
		Response: map[string]*ResponseInfo{},
	})

	got := make([]int, 0, len(results))
	for _, r := range results {
		got = append(got, r.StatusCode)
	}
	sort.Ints(got)

	want := []int{401, 404, 500}
	if len(got) != len(want) {
		t.Fatalf("expected %d responses, got %d: %v", len(want), len(got), got)
	}
	for i, st := range want {
		if got[i] != st {
			t.Fatalf("expected statuses %v, got %v", want, got)
		}
	}
}

// TestExtractResponse_SingleAssignment_NoFanOut ensures the fan-out path
// does not fire for variables with a single assignment, preserving
// pre-#39 latest-wins behaviour byte-for-byte.
func TestExtractResponse_SingleAssignment_NoFanOut(t *testing.T) {
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		Packages:   map[string]*metadata.Package{},
	}
	cfg := &APISpecConfig{
		Defaults: Defaults{ResponseContentType: "application/json"},
	}
	cp := NewContextProvider(meta)

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			cfg:             cfg,
			contextProvider: cp,
			schemaMapper:    NewSchemaMapper(cfg),
		},
		pattern: ResponsePattern{
			StatusFromArg:  true,
			StatusArgIndex: 0,
			DefaultStatus:  200,
		},
	}

	// Literal status arg: classic single-response flow.
	statusLit := metadata.NewCallArgument(meta)
	statusLit.SetKind(metadata.KindLiteral)
	statusLit.SetValue("201")
	edge := &metadata.CallGraphEdge{
		Args: []*metadata.CallArgument{statusLit},
	}

	results := matcher.ExtractResponse(&TrackerNode{CallGraphEdge: edge}, &RouteInfo{
		Response: map[string]*ResponseInfo{},
	})
	if len(results) != 1 {
		t.Fatalf("expected exactly 1 response for literal status, got %d", len(results))
	}
	if results[0].StatusCode != 201 {
		t.Fatalf("expected status 201, got %d", results[0].StatusCode)
	}
}
