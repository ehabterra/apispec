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

func TestFindTypeAndLookupFieldType(t *testing.T) {
	pool := metadata.NewStringPool()
	field := func(name, typ string) metadata.Field {
		return metadata.Field{Name: pool.Get(name), Type: pool.Get(typ)}
	}
	base := &metadata.Type{Fields: []metadata.Field{field("createdAt", "time.Time")}}
	user := &metadata.Type{
		Fields: []metadata.Field{field("id", "string"), field("age", "int")},
		Embeds: []int{pool.Get("example.com/app.Base")},
	}
	meta := &metadata.Metadata{
		StringPool: pool,
		Packages: map[string]*metadata.Package{
			"example.com/app": {
				Types: map[string]*metadata.Type{"User": user, "Base": base},
				Files: map[string]*metadata.File{
					"f.go": {Types: map[string]*metadata.Type{"Local": {Fields: []metadata.Field{field("x", "bool")}}}},
				},
			},
		},
	}

	// findType: package-level, file-level fallback, and misses.
	if findType(meta, "example.com/app", "User") == nil {
		t.Error("findType(User) should be found at package level")
	}
	if findType(meta, "example.com/app", "Local") == nil {
		t.Error("findType(Local) should be found at file level")
	}
	if findType(meta, "example.com/app", "Nope") != nil {
		t.Error("findType(Nope) should be nil")
	}
	if findType(meta, "", "User") != nil || findType(meta, "missing", "User") != nil {
		t.Error("findType with empty/missing pkg should be nil")
	}

	// lookupFieldType: direct field, embedded (one level deep), and miss.
	if got := lookupFieldType(meta, "example.com/app", "User", "id"); got != "string" {
		t.Errorf("lookupFieldType(id) = %q, want string", got)
	}
	if got := lookupFieldType(meta, "example.com/app", "User", "createdAt"); got != "time.Time" {
		t.Errorf("lookupFieldType(createdAt, embedded) = %q, want time.Time", got)
	}
	if got := lookupFieldType(meta, "example.com/app", "User", "missing"); got != "" {
		t.Errorf("lookupFieldType(missing) = %q, want empty", got)
	}
	if got := lookupFieldType(nil, "p", "T", "f"); got != "" {
		t.Errorf("lookupFieldType(nil meta) = %q", got)
	}
	if got := lookupFieldType(meta, "example.com/app", "Nope", "id"); got != "" {
		t.Errorf("lookupFieldType(unknown type) = %q", got)
	}
}
