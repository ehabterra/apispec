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
	"reflect"
	"sort"
)

// ThinOperation is an operation that is documented but whose response says
// nothing: content is present and the schema under it is empty.
type ThinOperation struct {
	Method string
	Path   string
	// Status is the response code whose schema came out empty.
	Status string
	// MediaType is the content type it was declared under.
	MediaType string
}

// thinOperations finds the responses that rendered as `application/json: {}` —
// content present, schema missing.
//
// This is the number issue #296 asked for, and the reason it asked in terms of
// ENDPOINTS rather than refused copies: on gitea the instance cap drops
// 25,267,970 call copies and costs nothing at all, so reporting the drops makes
// a harmless run look catastrophic while a genuinely starved response hides in
// the same message. Listing the scopes the cap fired in does not help either —
// it fires in nearly every scope, by design, because bounding a diamond is its
// job.
//
// So the honest pairing is: how many copies were refused, AND how many
// operations ended up saying nothing. Zero of the second is the useful, common
// answer, and until now a consumer could only get it by grepping the generated
// YAML for `schema: {}` and writing their own assertion around the count.
//
// Deliberately measured on the OUTPUT rather than inferred from the tree: what
// matters is what the document says, and a response can come out empty for
// reasons that have nothing to do with a budget. The report pairs the two
// numbers and lets the reader draw the line, rather than claiming a cause it
// cannot prove.
func thinOperations(spec *OpenAPISpec) []ThinOperation {
	if spec == nil {
		return nil
	}
	var out []ThinOperation
	for path, item := range spec.Paths {
		for method, op := range map[string]*Operation{
			"GET": item.Get, "POST": item.Post, "PUT": item.Put, "DELETE": item.Delete,
			"PATCH": item.Patch, "OPTIONS": item.Options, "HEAD": item.Head,
		} {
			if op == nil {
				continue
			}
			for status, resp := range op.Responses {
				for media, content := range resp.Content {
					if !schemaSaysNothing(content.Schema) {
						continue
					}
					out = append(out, ThinOperation{
						Method: method, Path: path, Status: status, MediaType: media,
					})
				}
			}
		}
	}
	// Map iteration reaches the output (golden rule #1).
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		if a.Status != b.Status {
			return a.Status < b.Status
		}
		return a.MediaType < b.MediaType
	})
	return out
}

// schemaSaysNothing reports whether a schema carries no information at all —
// the `application/json: {}` this metric is defined by.
//
// Compared against the ZERO schema rather than against a list of fields. The
// list version missed `Enum`, which a configured TypeMapping can return on its
// own, and would have missed `Format`, `Description` and every field added
// later — each a false positive reported as a lost response body, in a report
// whose whole value is that its usual answer is zero (CodeRabbit on #504).
//
// Runs once per response media entry at the end of a generation, so the
// reflection costs nothing worth naming.
func schemaSaysNothing(s *Schema) bool {
	return s == nil || reflect.DeepEqual(s, &Schema{})
}
