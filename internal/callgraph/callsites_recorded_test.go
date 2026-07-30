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

package callgraph

import (
	"go/types"
	"path/filepath"
	"testing"

	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/ssa"
)

// loadRecordedFixture builds the resolved graph for a testdata project.
func loadRecordedFixture(t *testing.T, name string) *Resolved {
	t.Helper()
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles |
			packages.NeedImports | packages.NeedTypes | packages.NeedTypesSizes |
			packages.NeedTypesInfo | packages.NeedSyntax | packages.NeedDeps,
		Dir: filepath.Join("..", "..", "testdata", name),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		t.Fatalf("load %s: %v", name, err)
	}
	return Build(pkgs)
}

// TestRecordedCalleeKeepsThePointerStar guards a defect that only appears when
// the resolved graph is used as a SOURCE of callee identity rather than as
// something to compare against.
//
// FunctionID strips the receiver's pointer star on purpose, so a call recorded on
// `*Encoder` joins to a function resolved on `Encoder`. Reading a callee back out
// of that ID therefore turns every pointer method into a value method — invisible
// in the call graph, because metadata's BaseID strips the star as well, and fatal
// in the extractor, where patterns match on the recorded receiver. Measured on the
// wrapper_router fixture when this split did not exist: every request body in the
// project disappeared.
//
// So FunctionID stays lossy (that is its job as a join key) and RecordedCallee
// renders the form metadata records. This pins that they differ in exactly the
// star and nowhere else.
func TestRecordedCalleeKeepsThePointerStar(t *testing.T) {
	r := loadRecordedFixture(t, "wrapper_router")

	var checked int
	for _, fns := range r.FunctionsAt() {
		for _, fn := range fns {
			if fn.Signature.Recv() == nil {
				continue
			}
			pkg, recvType, name := RecordedCallee(fn)
			if pkg == "" {
				continue
			}
			id := FunctionID(fn)
			if id == "" {
				continue
			}
			checked++

			if want := pkg + "." + trimLeadingStar(recvType) + "." + name; want != id {
				t.Errorf("RecordedCallee(%s) = (%q, %q, %q), which reassembles to %q rather than the FunctionID %q",
					fn, pkg, recvType, name, want, id)
			}
			if isPointerReceiver(fn) && !hasLeadingStar(recvType) {
				t.Errorf("RecordedCallee(%s) dropped the pointer star: recv=%q — a pointer method recorded as a value method matches no receiver-scoped pattern", fn, recvType)
			}
			if !isPointerReceiver(fn) && hasLeadingStar(recvType) {
				t.Errorf("RecordedCallee(%s) invented a pointer star: recv=%q", fn, recvType)
			}
		}
	}
	if checked == 0 {
		t.Fatal("no methods reached; the fixture is not exercising the receiver path")
	}
}

// TestRecordedCalleeHandlesTheAbsentCases covers what callers pass on a bad day.
func TestRecordedCalleeHandlesTheAbsentCases(t *testing.T) {
	if pkg, recv, name := RecordedCallee(nil); pkg != "" || recv != "" || name != "" {
		t.Errorf("RecordedCallee(nil) = (%q, %q, %q), want empties", pkg, recv, name)
	}
	var nilResolved *Resolved
	if got := nilResolved.FunctionsAt(); got != nil {
		t.Errorf("FunctionsAt on a nil graph = %v, want nil", got)
	}
	if got := (&Resolved{}).FunctionsAt(); got != nil {
		t.Errorf("FunctionsAt on an empty graph = %v, want nil", got)
	}
}

func trimLeadingStar(s string) string {
	if hasLeadingStar(s) {
		return s[1:]
	}
	return s
}

func hasLeadingStar(s string) bool { return len(s) > 0 && s[0] == '*' }

func isPointerReceiver(fn *ssa.Function) bool {
	recv := fn.Signature.Recv()
	if recv == nil {
		return false
	}
	_, isPtr := recv.Type().(*types.Pointer)
	return isPtr
}
