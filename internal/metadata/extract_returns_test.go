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
	"testing"
)

// TestExtractReturns_SkipsNestedClosures guards against a nested func literal's
// return statements leaking into the enclosing function's Returns (which would
// then pollute status extraction over that function). The handler-factory shape
// — a function returning a closure that has its own internal returns — is the
// canonical case.
func TestExtractReturns_SkipsNestedClosures(t *testing.T) {
	src := `package p
type E struct{ Code int }
func Factory() func() E {
	return func() E {
		if false {
			return E{Code: 400}
		}
		return E{Code: 404}
	}
}`
	file, info, fset := sweepTypeCheck(t, src)
	meta := &Metadata{StringPool: NewStringPool()}

	var body *ast.BlockStmt
	for _, d := range file.Decls {
		if fn, ok := d.(*ast.FuncDecl); ok && fn.Name.Name == "Factory" {
			body = fn.Body
		}
	}
	if body == nil {
		t.Fatal("Factory declaration not found")
	}

	_, allReturns := extractReturns(body, info, "p", fset, meta)

	// Factory has exactly one return of its own — `return func() E {…}`. The two
	// returns inside the closure must NOT be recorded here.
	if len(allReturns) != 1 {
		t.Fatalf("Factory.Returns = %d entries, want 1; the nested closure's returns leaked", len(allReturns))
	}
	// And that single return is the func literal, never the closure's `E{…}`.
	if len(allReturns[0]) != 1 || allReturns[0][0].GetKind() == KindCompositeLit {
		t.Errorf("Factory's return should be the func literal, not a struct composite from the closure body")
	}
}
