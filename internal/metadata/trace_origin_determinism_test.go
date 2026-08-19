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
	// The sorted scan reaches builder/aux.go first, which declares no method,
	// so the trace stops at the local assignment instead of following the
	// builder's receiver. Pinned, not just compared run to run: a flip is only
	// caught here if some run lands on the other order.
	//
	// That the receiver is NOT followed is issue #380 — the method lookup
	// caches its miss package-wide after scanning one file — so this value is
	// also a change detector: when #380 is fixed the answer becomes the
	// builder's receiver and this expectation moves with it.
	want := answer{variable: "cmd", pkg: "traceorigin/svc", caller: "Build"}
	for run := 0; run < 12; run++ {
		meta := generateMetaOnce(t, cfg)
		v, p, _, caller := metadata.TraceVariableOrigin("cmd", "Build", "traceorigin/svc", meta)
		if got := (answer{v, p, caller}); got != want {
			t.Fatalf("run %d traced `cmd` to %+v, want %+v", run, got, want)
		}
	}
}
