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
	"sync"
	"testing"

	metadata "github.com/ehabterra/apispec/internal/metadata"
)

// TestTypeRefOf covers the memoized structured accessor: one shared parse per
// pool id, safe under concurrency, structurally correct, and nil-safe.
func TestTypeRefOf(t *testing.T) {
	pool := metadata.NewStringPool()
	meta := &metadata.Metadata{StringPool: pool}

	id := pool.Get("[]*main.User")

	ref := meta.TypeRefOf(id)
	if ref == nil {
		t.Fatal("TypeRefOf returned nil for a valid pool id")
	}
	if core := ref.Core(); core.Pkg != "main" || core.Name != "User" {
		t.Errorf("core = %+v, want main.User", core)
	}

	// Memoization: the same pool id yields the same shared ref.
	if again := meta.TypeRefOf(id); again != ref {
		t.Error("TypeRefOf did not return the memoized ref")
	}

	// Concurrent access: every goroutine must observe one shared ref per id.
	otherID := pool.Get("map[string]main.Order")
	var wg sync.WaitGroup
	refs := make([]bool, 32)
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r1 := meta.TypeRefOf(id)
			r2 := meta.TypeRefOf(otherID)
			refs[i] = r1 == ref && r2 != nil
		}(i)
	}
	wg.Wait()
	for i, ok := range refs {
		if !ok {
			t.Fatalf("goroutine %d observed a different ref", i)
		}
	}

	// Nil-safety.
	var nilMeta *metadata.Metadata
	if nilMeta.TypeRefOf(0) != nil {
		t.Error("nil metadata must yield nil")
	}
	if meta.TypeRefOf(-1) != nil {
		t.Error("negative id must yield nil")
	}

	// CallArgument accessors ride on the same cache.
	arg := &metadata.CallArgument{Meta: meta, Type: id}
	if got := arg.TypeRef(); got != ref {
		t.Error("CallArgument.TypeRef must return the shared memoized ref")
	}
	var nilArg *metadata.CallArgument
	if nilArg.TypeRef() != nil || nilArg.ResolvedTypeRef() != nil {
		t.Error("nil argument must yield nil refs")
	}

	// Clone protects the cache from mutation.
	clone := ref.Clone()
	clone.Core().Name = "Mutated"
	if meta.TypeRefOf(id).Core().Name != "User" {
		t.Error("mutating a clone must not affect the cached ref")
	}
}
