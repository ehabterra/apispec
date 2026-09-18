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

import "sort"

// TruncatedOperation is one endpoint that came out with less than the walk
// could have found, and which limit did it.
type TruncatedOperation struct {
	// Method and Path identify the operation as a reader knows it. Both may be
	// empty when the registration produced no operation at all, in which case
	// Registration is the only handle there is.
	Method string
	Path   string

	// Registration is the call site the tree scoped the route to, which is what
	// the limits are actually counted against.
	Registration string

	// Limit says which budget fired, so the reader reaches for the right flag.
	// One value today — the instance cap is not reported this way, because it
	// fires in nearly every scope and what it costs is measured on the document
	// instead (ThinOperations).
	Limit string
}

// TruncatedByRouteBudget is the Limit value for --max-nodes-per-route, exported
// so a consumer can branch on it without matching prose.
const TruncatedByRouteBudget = "per-route node budget"

// truncatedOperations joins the registrations a limit cut short to the
// operations they produced.
//
// The tree counts against REGISTRATIONS, because that is the unit it scopes by;
// a reader thinks in endpoints. Until this existed a run reported "truncated 8
// of 1265 route subtrees" and named one of the eight, so discovering that
// `/{username}/{reponame}/compare` had quietly lost its `sort` and `template`
// query parameters took a four-point sweep of --max-instances-per-key, a diff of
// two specs, and a control run with a raised per-route budget (#296, #503).
//
// Joined on the node KEY, which both sides derive the same way: the tree names a
// scope by the key of the node that opened it, and a route remembers the node
// the extractor matched.
//
// THE JOIN IS PARTIAL BY NATURE, and the wording says so rather than
// overclaiming. Which node matches a route depends on how deep the walk got: for
// a router wrapper the match can be the registration inside the wrapper BODY,
// which is one node shared by every route that wrapper registers. So a truncated
// scope may have no route whose matched node is it, even though routes did come
// from it. Those are reported as the registration alone — "operation not
// attributed", not "no operation emitted", because the second would be a claim
// this cannot support.
//
// A scope that produced several operations — one per verb, after multi-verb
// expansion or `switch r.Method` splitting — is reported once per operation,
// because each is a separate line in the document and each came out thin.
func truncatedOperations(routes []*RouteInfo, routeScopes []int32, keyOf func(int32) string) []TruncatedOperation {
	if len(routeScopes) == 0 {
		return nil
	}

	byKey := map[string][]*RouteInfo{}
	for _, r := range routes {
		if r != nil && r.Node != nil {
			byKey[r.Node.GetKey()] = append(byKey[r.Node.GetKey()], r)
		}
	}

	var out []TruncatedOperation
	seen := map[TruncatedOperation]bool{}
	add := func(scopes []int32, limit string) {
		for _, id := range scopes {
			reg := keyOf(id)
			matches := byKey[reg]
			if len(matches) == 0 {
				op := TruncatedOperation{Registration: reg, Limit: limit}
				if !seen[op] {
					seen[op] = true
					out = append(out, op)
				}
				continue
			}
			for _, r := range matches {
				op := TruncatedOperation{
					Method:       r.Method,
					Path:         r.OpenAPIPath(),
					Registration: reg,
					Limit:        limit,
				}
				if !seen[op] {
					seen[op] = true
					out = append(out, op)
				}
			}
		}
	}
	add(routeScopes, TruncatedByRouteBudget)

	// Sorted for the output's sake (golden rule #1): scope order is stable per
	// tree, but the routes a scope maps to come from a map, and this reaches
	// stderr and the diagnostics alike.
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if a.Limit != b.Limit {
			return a.Limit < b.Limit
		}
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Method != b.Method {
			return a.Method < b.Method
		}
		return a.Registration < b.Registration
	})
	return out
}
