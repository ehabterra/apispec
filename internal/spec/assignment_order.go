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
	"cmp"
	"slices"

	"github.com/ehabterra/apispec/internal/metadata"
)

// sortedAssignmentRelationships returns the metadata's assignment links in a
// total order, for both trackers to consume.
//
// Order is load-bearing, not cosmetic: the consumers write the links into
// last-write-wins indexes (assignmentIndex/assignIndex[akey] = producer) and
// append to per-producer child lists, so it decides which producer an
// ambiguous variable resolves to and in which order a node's children are
// expanded — and under a truncation budget, expansion order decides what
// survives. The source is a map, so it is randomised per run.
//
// The producing call's instance ID alone does NOT order the list: one edge can
// be linked to several assignments (`a, b := f()`, an edge whose AssignmentMap
// holds more than one variable), and the sort is unstable, so those ties were
// broken by the random map order — 9,284 of 21,735 links on gitea sat in such a
// tie, which is how the same command produced different specs run to run
// (issue #340). The link's AssignmentKey is the map key, hence unique, so
// tie-breaking on its fields is a total order.
func sortedAssignmentRelationships(meta *metadata.Metadata) []*metadata.AssignmentLink {
	relationships := meta.GetAssignmentRelationships()
	rels := make([]*metadata.AssignmentLink, 0, len(relationships))
	for _, rel := range relationships {
		rels = append(rels, rel)
	}
	slices.SortFunc(rels, func(a, b *metadata.AssignmentLink) int {
		ak, bk := a.AssignmentKey, b.AssignmentKey
		return cmp.Or(
			cmp.Compare(a.Edge.Callee.ID(), b.Edge.Callee.ID()),
			cmp.Compare(ak.Container, bk.Container),
			cmp.Compare(ak.Pkg, bk.Pkg),
			cmp.Compare(ak.Name, bk.Name),
			cmp.Compare(ak.Type, bk.Type),
		)
	})
	return rels
}
