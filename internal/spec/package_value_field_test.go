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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// A declaration and the literal initialising it start on the same LINE at
// different columns, which is how one is matched to the other — so the column
// has to come off, and a position that carries no line has to be refused rather
// than matched against everything.
func TestFileLine(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"/a/b/hdr.go:11:5", "/a/b/hdr.go:11", true},
		{"/a/b/hdr.go:11:14", "/a/b/hdr.go:11", true}, // the literal, same line
		{"hdr.go:3:1", "hdr.go:3", true},
		// Refused: nothing to match on.
		{"hdr.go", "", false},
		{"hdr.go:11", "", false}, // a line with no column is not the recorded form
		{"", "", false},
		{":", "", false},
	}
	for _, tc := range cases {
		got, ok := fileLine(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Errorf("fileLine(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

// A declaration and its literal share a line, so two declarations sharing one
// line cannot be told apart — `var A, B = T{…}, T{…}` — and the honest answer
// is to resolve neither (golden rule #7).
func TestFieldOfInstanceAtRequiresOneLiteral(t *testing.T) {
	pool := newTestPool()
	impl := &ContextProviderImpl{meta: metaWithPool(pool)}

	file := fileWithInstances(pool,
		instanceAt(pool, "hdr.go:11:14", map[string]string{"Key": "X-A"}, map[string]string{"Key": "literal"}),
	)
	if got, ok := fieldOfInstanceAt(impl, file, "hdr.go:11:5", "Key"); got != "X-A" || !ok {
		t.Errorf("one literal on the line = (%q, %v), want (\"X-A\", true)", got, ok)
	}

	// A second literal on the same line: which one the declaration means is not
	// stated, so neither answers.
	two := fileWithInstances(pool,
		instanceAt(pool, "hdr.go:11:14", map[string]string{"Key": "X-A"}, map[string]string{"Key": "literal"}),
		instanceAt(pool, "hdr.go:11:40", map[string]string{"Key": "X-B"}, map[string]string{"Key": "literal"}),
	)
	if got, ok := fieldOfInstanceAt(impl, two, "hdr.go:11:5", "Key"); ok {
		t.Errorf("two literals on one line resolved to %q; which one is not stated", got)
	}

	// A field set from a CALL is not a value: Fields holds the expression
	// rendered, and reading it back would document `func() string()` as a
	// header name (#452).
	call := fileWithInstances(pool,
		instanceAt(pool, "hdr.go:11:14", map[string]string{"Key": "func() string()"}, map[string]string{"Key": "call"}),
	)
	if got, ok := fieldOfInstanceAt(impl, call, "hdr.go:11:5", "Key"); ok {
		t.Errorf("a field set from a call resolved to %q", got)
	}

	// A field the literal never sets holds the zero value, which for a name
	// means there is none — reported as resolved-to-empty, and dropped
	// downstream.
	if got, ok := fieldOfInstanceAt(impl, file, "hdr.go:11:5", "Absent"); got != "" || !ok {
		t.Errorf("an unset field = (%q, %v), want (\"\", true)", got, ok)
	}

	// No literal on that line at all.
	if _, ok := fieldOfInstanceAt(impl, file, "hdr.go:99:5", "Key"); ok {
		t.Error("a line with no literal resolved")
	}
	// And an unusable position.
	if _, ok := fieldOfInstanceAt(impl, file, "", "Key"); ok {
		t.Error("an empty position resolved")
	}
}

func newTestPool() *metadata.StringPool { return metadata.NewStringPool() }

func metaWithPool(pool *metadata.StringPool) *metadata.Metadata {
	return &metadata.Metadata{StringPool: pool}
}

func fileWithInstances(pool *metadata.StringPool, instances ...metadata.StructInstance) *metadata.File {
	return &metadata.File{StructInstances: instances}
}

func instanceAt(pool *metadata.StringPool, pos string, fields, kinds map[string]string) metadata.StructInstance {
	si := metadata.StructInstance{
		Position:   pool.Get(pos),
		Fields:     map[int]int{},
		FieldKinds: map[int]int{},
	}
	for name, value := range fields {
		key := pool.Get(name)
		si.Fields[key] = pool.Get(value)
		si.FieldKinds[key] = pool.Get(kinds[name])
	}
	return si
}
