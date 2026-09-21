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

package metadata

import (
	"go/ast"
	"go/types"
	"testing"
)

// A method call records what it is made ON, for the shapes ChainParent does
// not cover. CalleeVarName kept only a bare identifier's name, so a call made
// on a FIELD (`req.Header.Set`) left no trace of its receiver, and a header
// written onto an outbound request could not be told from the response's own
// (issue #543).
func TestCallEdgeRecordsReceiver(t *testing.T) {
	file, info, fset := sweepTypeCheck(t, `package p

import "fmt"

type Header map[string][]string

func (h Header) Set(k, v string) {}

type Request struct{ Header Header }

type Writer struct{}

func (Writer) Header() Header { return nil }

func handle(w Writer, req *Request) {
	req.Header.Set("a", "field")
	w.Header().Set("a", "chained")
	h := Header{}
	h.Set("a", "ident")
	fmt.Println("qualified")
}
`)
	meta := GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"sweep.go": file}},
		map[*ast.File]*types.Info{file: info},
		map[string]string{"sweep.go": "p"},
		fset,
	)

	// Keyed by the call's first literal argument, which names the shape.
	byShape := map[string]*CallGraphEdge{}
	var header *CallGraphEdge
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		name := meta.StringPool.GetString(e.Callee.Name)
		if name == "Header" {
			header = e
		}
		if len(e.Args) > 0 {
			v := e.Args[len(e.Args)-1].GetValue()
			if len(v) > 2 {
				byShape[v[1:len(v)-1]] = e
			}
		}
	}

	field := byShape["field"]
	if field == nil || field.Receiver == nil {
		t.Fatalf("req.Header.Set recorded no receiver: %+v", field)
	}
	if field.Receiver.GetKind() != KindSelector || field.Receiver.X.GetName() != "req" ||
		field.Receiver.Sel.GetName() != "Header" {
		t.Errorf("req.Header.Set receiver = %s %q, want the selector req.Header",
			field.Receiver.GetKind(), CallArgToString(field.Receiver))
	}

	if ident := byShape["ident"]; ident == nil || ident.Receiver == nil || ident.Receiver.GetName() != "h" {
		t.Errorf("h.Set receiver = %+v, want the ident h", ident)
	}

	// A chained call's receiver is its ChainParent, not a re-rendered chain.
	if chained := byShape["chained"]; chained == nil || chained.Receiver != nil || chained.ChainParent == nil {
		t.Errorf("w.Header().Set should carry its receiver as ChainParent only, got %+v", chained)
	}
	if header == nil || header.Receiver == nil || header.Receiver.GetName() != "w" {
		t.Errorf("w.Header() receiver = %+v, want the ident w", header)
	}

	// A package qualifier is not a receiver.
	if q := byShape["qualified"]; q == nil || q.Receiver != nil {
		t.Errorf("fmt.Println recorded a receiver: %+v", q)
	}
}

// Without type information, a bare qualifier that the file's own scope does not
// declare cannot be shown to be a value, so it is not recorded as a receiver —
// losing one only leaves a receiver unknown, never wrong.
func TestMethodReceiverWithoutTypeInfo(t *testing.T) {
	file, fset := sweepParseFile(t, `package p

import "fmt"

func f() {
	x := struct{}{}
	fmt.Println(x)
}
`)
	meta := &Metadata{StringPool: NewStringPool()}
	var got = map[string]*CallArgument{}
	ast.Inspect(file, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
				got[sel.Sel.Name] = methodReceiver(sel, nil, "p", fset, meta)
			}
		}
		return true
	})
	if r, ok := got["Println"]; !ok || r != nil {
		t.Errorf("fmt.Println without type info recorded receiver %+v", r)
	}
}

// A call records the static type of its single result, which is the only
// record of what an external constructor returns: a dependency's declarations
// are not loaded (issue #556). A tuple records nothing.
func TestCallEdgeRecordsResultType(t *testing.T) {
	file, info, fset := sweepTypeCheck(t, `package p

type HTTPError struct{ Code int }

func (e *HTTPError) Error() string { return "" }

func NewHTTPError(code int) *HTTPError { return &HTTPError{Code: code} }

func pair() (int, error) { return 0, nil }

func handle() error {
	_, _ = pair()
	return NewHTTPError(404)
}
`)
	meta := GenerateMetadata(
		map[string]map[string]*ast.File{"p": {"sweep.go": file}},
		map[*ast.File]*types.Info{file: info},
		map[string]string{"sweep.go": "p"},
		fset,
	)
	got := map[string]string{}
	for i := range meta.CallGraph {
		e := &meta.CallGraph[i]
		got[meta.StringPool.GetString(e.Callee.Name)] = e.ResultType
	}
	if got["NewHTTPError"] != "*p.HTTPError" {
		t.Errorf("NewHTTPError result type = %q, want *p.HTTPError", got["NewHTTPError"])
	}
	if got["pair"] != "" {
		t.Errorf("a tuple-returning call recorded %q", got["pair"])
	}
}
