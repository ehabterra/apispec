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

import "testing"

func TestPathParamConstraints(t *testing.T) {
	cases := []struct {
		name, path, param string
		typ, format       string
		pattern           string
		minLen, maxLen    int
		min, max          float64
		described         bool
	}{
		{name: "int", path: "/items/:id<int>", param: "id", typ: "integer", described: true},
		{name: "bool", path: "/f/:on<bool>", param: "on", typ: "boolean", described: true},
		{name: "float", path: "/f/:x<float>", param: "x", typ: "number", described: true},
		{name: "guid", path: "/u/:uid<guid>", param: "uid", typ: "string", format: "uuid", described: true},
		{name: "uuid spelling", path: "/u/:uid<uuid>", param: "uid", typ: "string", format: "uuid", described: true},
		{name: "alpha", path: "/a/:w<alpha>", param: "w", typ: "string", pattern: "^[A-Za-z]+$", described: true},

		{name: "minLen", path: "/a/:w<minLen(3)>", param: "w", typ: "string", minLen: 3, described: true},
		{name: "maxLen", path: "/a/:w<maxLen(9)>", param: "w", typ: "string", maxLen: 9, described: true},
		{name: "len fills both bounds", path: "/a/:w<len(4)>", param: "w", typ: "string", minLen: 4, maxLen: 4, described: true},
		{name: "range", path: "/p/:n<range(1,10)>", param: "n", typ: "integer", min: 1, max: 10, described: true},
		{name: "chained", path: "/p/:n<min(1);max(500)>", param: "n", typ: "integer", min: 1, max: 500, described: true},
		{name: "regex arg", path: "/p/:s<regex([a-z]+)>", param: "s", typ: "string", pattern: "[a-z]+", described: true},

		// A ';' inside the argument does not split the list.
		{name: "semicolon inside regex", path: "/p/:s<regex(a;b)>", param: "s", typ: "string", pattern: "a;b", described: true},

		// Nothing is claimed: the layout decides date vs date-time, which is a
		// parse rather than a table row.
		{name: "datetime states nothing", path: "/e/:d<datetime(2006-01-02)>", param: "d", typ: "string"},
		{name: "unknown constraint", path: "/o/:x<mystery(7)>", param: "x", typ: "string"},
		// Wrong arity states nothing extra rather than half-filling the schema.
		{name: "bad argument", path: "/p/:n<min(abc)>", param: "n", typ: "integer", described: true},

		// The mux/chi regex form keeps today's behaviour: a pattern, and
		// `described` false, because a regex says what the value looks like
		// without saying what it is (that is #345).
		{name: "mux regex", path: "/items/{id:[0-9]+}", param: "id", typ: "string", pattern: "[0-9]+"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := pathParamConstraints(tc.path)[tc.param]
			if !ok {
				t.Fatalf("pathParamConstraints(%q) has no entry for %q", tc.path, tc.param)
			}
			s := got.clone()
			if s.Type != tc.typ || s.Format != tc.format || s.Pattern != tc.pattern {
				t.Errorf("schema = {type:%q format:%q pattern:%q}, want {type:%q format:%q pattern:%q}",
					s.Type, s.Format, s.Pattern, tc.typ, tc.format, tc.pattern)
			}
			if s.MinLength != tc.minLen || s.MaxLength != tc.maxLen {
				t.Errorf("length bounds = [%d,%d], want [%d,%d]", s.MinLength, s.MaxLength, tc.minLen, tc.maxLen)
			}
			if s.Minimum != tc.min || s.Maximum != tc.max {
				t.Errorf("value bounds = [%v,%v], want [%v,%v]", s.Minimum, s.Maximum, tc.min, tc.max)
			}
			if got.described != tc.described {
				t.Errorf("described = %v, want %v", got.described, tc.described)
			}
		})
	}
}

// A path with nothing to say produces nothing, and an unterminated tail is not
// read at all rather than being read to the end of the path.
func TestPathParamConstraintsEmptyAndUnterminated(t *testing.T) {
	if got := pathParamConstraints("/plain/{name}"); got != nil {
		t.Errorf("unconstrained path produced %v", got)
	}
	if got := pathParamConstraints("/x/:id<int"); got != nil {
		t.Errorf("unterminated constraint produced %v — nothing about it is reliable", got)
	}
}

// clone must hand back a copy: one constraint feeds every operation that has
// the placeholder, and a shared pointer would let one operation's later edit
// rewrite another's parameter.
func TestPathParamConstraintCloneIsACopy(t *testing.T) {
	c := pathParamConstraints("/items/:id<int>")["id"]
	a, b := c.clone(), c.clone()
	a.Type = "string"
	if b.Type != "integer" {
		t.Errorf("clone shares state: b.Type = %q after editing a", b.Type)
	}
	var zero pathParamConstraint
	if s := zero.clone(); s == nil || s.Type != "string" {
		t.Errorf("the zero constraint must clone to the general type, got %+v", s)
	}
}
