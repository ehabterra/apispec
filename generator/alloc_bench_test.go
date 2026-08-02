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

package generator

import (
	"path/filepath"
	"testing"

	"github.com/ehabterra/apispec/internal/engine"
)

// BenchmarkPipelineAlloc exists for ONE number: B/op, which comes from runtime
// counters and is therefore exact and comparable across builds. It is the tool
// for A/B-ing a change that alters how much memory the pipeline allocates —
// struct layout, buffer reuse, an extra copy.
//
// Do NOT read ns/op here. On a fixture this small the run is dominated by
// go/packages loading, which swamps anything the pipeline itself does; the
// per-stage `[engine]` timings on a real project are the honest speed signal
// (see the profiling notes in CLAUDE.md). Heap PROFILES are no use for this
// either: they are sampled, so a few percent on one line disappears into the
// sampling error.
//
// Usage:
//
//	go test ./generator/ -run XXX -bench BenchmarkPipelineAlloc -benchmem -count=5
func BenchmarkPipelineAlloc(b *testing.B) {
	// dense_graph exercises tree expansion (the node-heavy path);
	// complex_chi_router exercises a realistic multi-package extraction.
	for _, fixture := range []string{"dense_graph", "complex_chi_router"} {
		b.Run(fixture, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				cfg := engine.DefaultEngineConfig()
				cfg.InputDir = filepath.Join("..", "testdata", fixture)
				if _, err := engine.NewEngine(cfg).GenerateOpenAPI(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
