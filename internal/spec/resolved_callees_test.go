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
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

func TestSplitTarget(t *testing.T) {
	tests := []struct {
		target string
		pkg    string
		recv   string
		name   string
	}{
		{"github.com/x/y.Type.Method", "github.com/x/y", "Type", "Method"},
		{"github.com/x/y.Func", "github.com/x/y", "", "Func"},
		{"app.Type.Method", "app", "Type", "Method"},
		{"app.Func", "app", "", "Func"},
		{"Bare", "", "", ""},
		{"", "", "", ""},
	}
	for _, tt := range tests {
		pkg, recv, name := splitTarget(tt.target)
		if pkg != tt.pkg || recv != tt.recv || name != tt.name {
			t.Errorf("splitTarget(%q) = (%q, %q, %q), want (%q, %q, %q)",
				tt.target, pkg, recv, name, tt.pkg, tt.recv, tt.name)
		}
	}
}

// TestRewriteCalleeDropsTheMemoizedIdentity is the trap this code exists around:
// Call memoizes its own BaseID, so patching Pkg or RecvType in place would leave
// the edge answering with the identity it had before.
func TestRewriteCalleeDropsTheMemoizedIdentity(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	callee := metadata.Call{
		Meta:     meta,
		Name:     pool.Get("List"),
		Pkg:      pool.Get("example.com/app"),
		RecvType: pool.Get("Storage"),
	}
	if before := callee.BaseID(); before != "example.com/app.Storage.List" {
		t.Fatalf("BaseID before = %q", before)
	}

	if !rewriteCallee(meta, &callee, "example.com/app.s3driver.List") {
		t.Fatal("rewriteCallee refused a well-formed target")
	}
	if got := callee.BaseID(); got != "example.com/app.s3driver.List" {
		t.Errorf("BaseID after rewrite = %q, want the resolved target — a stale memo would return the old identity forever", got)
	}

	// A target that is not a function ID must be refused rather than half-applied.
	if rewriteCallee(meta, &callee, "nodots") {
		t.Error("rewriteCallee accepted a target with no package")
	}
}

// TestApplyResolvedCalleesIsSafeWithoutAGraph pins that the step is a no-op when
// the resolved graph is off, which is its default.
func TestApplyResolvedCalleesIsSafeWithoutAGraph(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	if stats := ApplyResolvedCallees(meta, nil, nil); stats.Joined != 0 {
		t.Errorf("a nil resolved graph joined %d sites", stats.Joined)
	}
	if stats := ApplyResolvedCallees(nil, nil, nil); stats.Joined != 0 {
		t.Errorf("nil metadata joined %d sites", stats.Joined)
	}
}

func TestResolvedCalleeStatsLine(t *testing.T) {
	line := ResolvedCalleeStats{Joined: 69493, Interface: 2880, Promoted: 4492, Ambiguous: 1570, Unexplained: 1240}.Line()
	for _, want := range []string{"69493", "7372", "2880", "4492", "1570", "1240"} {
		if !strings.Contains(line, want) {
			t.Errorf("stats line %q does not report %s", line, want)
		}
	}
}

// TestApplyResolvedCalleesClassRules covers the decision this migration turns
// on: which disagreements are acted on and which are left exactly as recorded.
func TestApplyResolvedCalleesClassRules(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}
	meta.Packages = map[string]*metadata.Package{
		"app": {Files: map[string]*metadata.File{
			"types.go": {Types: map[string]*metadata.Type{
				"Storage": {Name: pool.Get("Storage"), Kind: pool.Get("interface")},
				"Base":    {Name: pool.Get("Base"), Kind: pool.Get("struct")},
				"Context": {Name: pool.Get("Context"), Kind: pool.Get("struct"), Embeds: []int{pool.Get("*Base")}},
				"Widget":  {Name: pool.Get("Widget"), Kind: pool.Get("struct")},
			}},
		}},
	}

	edge := func(position, pkg, recv, name string) metadata.CallGraphEdge {
		return metadata.CallGraphEdge{
			Position: pool.Get(position),
			Caller:   metadata.Call{Meta: meta, Name: pool.Get("caller"), Pkg: pool.Get("app")},
			Callee: metadata.Call{
				Meta: meta, Name: pool.Get(name), Pkg: pool.Get(pkg), RecvType: pool.Get(recv),
			},
		}
	}

	meta.CallGraph = []metadata.CallGraphEdge{
		edge("f.go:1:1", "app", "Storage", "List"), // one implementation -> rewritten
		edge("f.go:2:1", "app", "Context", "Form"), // promoted           -> rewritten
		edge("f.go:3:1", "app", "Storage", "Save"), // several impls      -> left alone
		edge("f.go:4:1", "app", "Widget", "Close"), // unexplained        -> left alone
		edge("f.go:5:1", "app", "Base", "Write"),   // agreement          -> untouched
		edge("f.go:6:1", "app", "Widget", "Draw"),  // no resolved site   -> untouched
	}

	byPosition := map[string][]string{
		"f.go:1#List":  {"app.s3driver.List"},
		"f.go:2#Form":  {"app.Base.Form"},
		"f.go:3#Save":  {"app.s3driver.Save", "app.localDriver.Save"},
		"f.go:4#Close": {"other.File.Close"},
		"f.go:5#Write": {"app.Base.Write"},
	}

	stats := applyResolvedCallees(meta, byPosition)

	if stats.Joined != 5 {
		t.Errorf("joined %d sites, want 5 (the sixth has no resolved call)", stats.Joined)
	}
	if stats.Interface != 1 || stats.Promoted != 1 {
		t.Errorf("rewrote %d interface / %d promoted, want 1 / 1", stats.Interface, stats.Promoted)
	}
	if stats.Ambiguous != 1 || stats.Unexplained != 1 {
		t.Errorf("left %d ambiguous / %d unexplained, want 1 / 1", stats.Ambiguous, stats.Unexplained)
	}

	want := []string{
		"app.s3driver.List", // resolved to the single implementation
		"app.Base.Form",     // resolved to the type declaring the promoted method
		"app.Storage.Save",  // ambiguous: stays the interface
		"app.Widget.Close",  // unexplained: stays exactly as recorded
		"app.Base.Write",    // agreement
		"app.Widget.Draw",   // never joined
	}
	for i, expected := range want {
		if got := meta.CallGraph[i].Callee.BaseID(); got != expected {
			t.Errorf("edge %d callee = %q, want %q", i, got, expected)
		}
	}
}

// TestRewriteCalleeCarriesTheWrittenReceiver is the change-detector for the
// second half of issue #260.
//
// Resolving is the act of replacing the interface a call names with the concrete
// type that runs. The pattern configs are scoped to those interface names, so a
// rewrite that DROPPED the written receiver would silently stop every
// interface-scoped pattern from matching and every interface-scoped exclusion
// from excluding — the defect being fixed, arriving through the rewrite rather
// than through the recorder.
func TestRewriteCalleeCarriesTheWrittenReceiver(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	callee := metadata.Call{
		Meta:     meta,
		Name:     pool.Get("Header"),
		Pkg:      pool.Get("net/http"),
		RecvType: pool.Get("ResponseWriter"),
	}
	if !rewriteCallee(meta, &callee, "net/http.*recorder.Header") {
		t.Fatal("rewriteCallee refused a well-formed target")
	}

	if got := pool.GetString(callee.RecvType); got != "*recorder" {
		t.Errorf("recorded receiver = %q, want the resolved concrete type", got)
	}
	written := callee.WrittenRecvType()
	if written < 0 {
		t.Fatal("the written receiver was dropped; every pattern scoped to net/http.ResponseWriter now fails to match")
	}
	if got := pool.GetString(written); got != "ResponseWriter" {
		t.Errorf("written receiver = %q, want the interface the call names in source", got)
	}

	// End of the chain: both forms must reach the matchers.
	forms := recvForms(NewContextProvider(meta), &callee)
	if len(forms) != 2 || forms[0] != "net/http.*recorder" || forms[1] != "net/http.ResponseWriter" {
		t.Errorf("recvForms = %v, want the concrete type then the written interface", forms)
	}
}

// TestRewriteCalleeDoesNotInventASecondForm keeps the carry honest: a rewrite
// that does not change the receiver has nothing to remember, and recording an
// identical second form would make every receiver test compare the same string
// twice.
func TestRewriteCalleeDoesNotInventASecondForm(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	// Same receiver, different package — an interface resolved to an
	// implementation that happens to share the type name.
	callee := metadata.Call{
		Meta:     meta,
		Name:     pool.Get("List"),
		Pkg:      pool.Get("example.com/app"),
		RecvType: pool.Get("Storage"),
	}
	if !rewriteCallee(meta, &callee, "example.com/drivers.Storage.List") {
		t.Fatal("rewriteCallee refused a well-formed target")
	}
	if callee.WrittenRecvType() >= 0 {
		t.Errorf("recorded a written receiver %q identical to the resolved one",
			pool.GetString(callee.WrittenRecvType()))
	}
	if forms := recvForms(NewContextProvider(meta), &callee); len(forms) != 1 {
		t.Errorf("recvForms = %v, want one form", forms)
	}
}
