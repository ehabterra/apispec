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

package metadata_test

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// traceOriginFixture reproduces the shape that made spec output vary run to run
// on a large project (issue #340): a builder call whose method lives in ONE file
// of a multi-file package.
//
// Resolving `cmd` follows the assignment into gitcmd-style package `builder`,
// where the lookup scans the package's files for the method. The scan used to
// range the Files map, so which file it saw first decided whether the method was
// found at all — and the answer is memoized for the rest of the run, so one coin
// flip during loading changed which producers the tracker resolved, which
// subtrees the barren-subtree prune kept, and therefore which responses and
// header parameters survived the instance cap.
var traceOriginFixture = testModule{
	Name: "traceorigin",
	Files: map[string]interface{}{
		"main.go": `package main

import "traceorigin/svc"

func main() { svc.Build() }
`,
		"svc/svc.go": `package svc

import "traceorigin/builder"

func Build() {
	cmd := builder.New().WithArgs("clone")
	builder.Run(cmd)
}
`,
		// Sorts before command.go, and declares no method at all: the file the
		// scan reaches first when file names are ordered.
		"builder/aux.go": `package builder

type Aux struct{ Note string }
`,
		"builder/command.go": `package builder

type Command struct{ Args []string }

func New() *Command { return &Command{} }

func (c *Command) WithArgs(args ...string) *Command {
	c.Args = append(c.Args, args...)
	return c
}

func Run(c *Command) error { return nil }
`,
	},
}

// TestTraceVariableOriginDeterministic asserts that tracing the same variable
// over freshly loaded metadata answers identically every time.
//
// Each iteration re-loads the packages, so Go's per-map iteration randomisation
// differs between them exactly as it does between real runs — an unordered file
// scan flips the answer here roughly half the time.
func TestTraceVariableOriginDeterministic(t *testing.T) {
	cfg := exportModules(t, []testModule{traceOriginFixture})

	type answer struct{ variable, pkg, caller string }
	// The trace follows `WithArgs` to the builder's receiver, which is the
	// producer the assignment names. Pinned, not just compared run to run: a
	// flip is only caught here if some run lands on the other file order.
	//
	// This expectation moved when #380 was fixed. It used to be the LOCAL
	// assignment — `{cmd, traceorigin/svc, Build}`, the variable resolving to
	// itself — because the method lookup scanned one file and cached its miss
	// for the whole package, and `builder/aux.go` (no methods) sorts before
	// `builder/command.go` (which declares the method). The fixture is built
	// that way on purpose: with the lookup now package-wide, the file order
	// that used to decide the answer no longer can.
	want := answer{variable: "c", pkg: "traceorigin/builder", caller: "WithArgs"}
	for run := 0; run < 12; run++ {
		meta := generateMetaOnce(t, cfg)
		v, p, _, caller := metadata.TraceVariableOrigin("cmd", "Build", "traceorigin/svc", meta)
		if got := (answer{v, p, caller}); got != want {
			t.Fatalf("run %d traced `cmd` to %+v, want %+v", run, got, want)
		}
	}
}

// methodLookupFileOrderFixture declares the method in the file that sorts
// FIRST, which is the arrangement the old lookup happened to get right.
//
// traceOriginFixture is the other one: there the method is in the second file
// and the trace stopped at the local assignment. Pinning both is the point —
// the defect was never "methods are not found", it was "methods are found only
// in the first file", so a fixture in either arrangement alone reports the bug
// as absent (issue #380).
var methodLookupFileOrderFixture = testModule{
	Name: "methodorder",
	Files: map[string]interface{}{
		"main.go": `package main

import "methodorder/svc"

func main() { svc.Build() }
`,
		"svc/svc.go": `package svc

import "methodorder/builder"

func Build() {
	cmd := builder.New().WithArgs("clone")
	builder.Run(cmd)
}
`,
		// "a_command.go" sorts BEFORE "zz_aux.go", so the method is in the
		// first file scanned.
		"builder/a_command.go": `package builder

type Command struct{ args []string }

func New() *Command { return &Command{} }

func (c *Command) WithArgs(args ...string) *Command {
	c.args = args
	return c
}

func Run(c *Command) {}
`,
		"builder/zz_aux.go": `package builder

const unused = "aux"
`,
	},
}

// The answer must not depend on which file declares the method.
//
// Both fixtures are the same program with the builder's two files renamed, so
// the only difference is sort order. Before #380 they disagreed: the method was
// found only when it sorted first, and the miss was then cached package-wide
// for the rest of the run.
func TestTraceVariableOriginIsIndependentOfFileOrder(t *testing.T) {
	type answer struct{ variable, pkg, caller string }
	trace := func(mod testModule, module string) answer {
		cfg := exportModules(t, []testModule{mod})
		meta := generateMetaOnce(t, cfg)
		v, p, _, caller := metadata.TraceVariableOrigin("cmd", "Build", module+"/svc", meta)
		return answer{v, p, caller}
	}

	methodSecond := trace(traceOriginFixture, "traceorigin")
	methodFirst := trace(methodLookupFileOrderFixture, "methodorder")

	// Same shape, so the same resolution — only the package name differs.
	if methodSecond.variable != methodFirst.variable || methodSecond.caller != methodFirst.caller {
		t.Errorf("file order changed the answer: method-in-second-file gave %+v, "+
			"method-in-first-file gave %+v — the lookup is per file again, and a method "+
			"declared outside the first file is invisible", methodSecond, methodFirst)
	}
	// And it is the producer, not the variable resolving to itself.
	if methodSecond.variable != "c" || methodSecond.caller != "WithArgs" {
		t.Errorf("traced to %+v, want the builder's receiver — a trace that stops at the "+
			"local assignment leaves the tracker with no producer to expand", methodSecond)
	}
}
