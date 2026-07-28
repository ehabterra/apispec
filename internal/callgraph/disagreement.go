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

package callgraph

import "strings"

// Disagreement classifies why the resolved graph names a different callee than
// the syntactic one at the same call site.
//
// The classes are not cosmetic: each one is a decision about whether the
// resolved answer may be trusted over the recorded one. An interface call
// resolved to its single implementation is an improvement; the same call
// resolved to four implementations is an ambiguity that must stay ambiguous
// (golden rule #7), and a target in an unrelated package is a join that landed
// on the wrong call and must not be acted on at all.
type Disagreement int

const (
	// DisagreeUnknown is a difference none of the classes below explain. Treat it
	// as untrustworthy: it is the shape a mis-join takes.
	DisagreeUnknown Disagreement = iota

	// DisagreeInterface: the recorded callee is a method on an interface and the
	// resolved one is a concrete implementation. This is what the resolved graph
	// exists for.
	DisagreeInterface

	// DisagreePromoted: the recorded callee is a method on a type that EMBEDS the
	// type declaring it, and the resolved one names the declaring type. Go
	// promotes the method, so both names describe the same function — gitea's
	// `ctx.FormString` recorded on `*Context`, declared on `*Base`.
	DisagreePromoted

	// DisagreeAmbiguous: several concrete targets at one site, none of which the
	// recorded name matches. Honest resolution keeps all of them.
	DisagreeAmbiguous
)

// String names the class for reports and test failures.
func (d Disagreement) String() string {
	switch d {
	case DisagreeInterface:
		return "interface->concrete"
	case DisagreePromoted:
		return "promoted-through-embedding"
	case DisagreeAmbiguous:
		return "ambiguous"
	default:
		return "unexplained"
	}
}

// TypeFacts is what classification needs to know about the program's types,
// supplied by the caller so this package does not depend on the metadata layer.
type TypeFacts struct {
	// IsInterface reports whether a fully-qualified type name (`pkg.Name`) is
	// declared as an interface.
	IsInterface func(qualified string) bool
	// Embeds reports whether the first type embeds the second, transitively.
	Embeds func(outer, inner string) bool
}

// Classify explains a disagreement between the callee a call site was recorded
// with and the targets the resolved graph gives for it.
func Classify(recorded string, resolved []string, facts TypeFacts) Disagreement {
	if len(resolved) == 0 {
		return DisagreeUnknown
	}
	if len(resolved) > 1 {
		return DisagreeAmbiguous
	}

	recordedOwner, recordedName := splitOwner(recorded)
	resolvedOwner, resolvedName := splitOwner(resolved[0])
	if recordedName == "" || recordedName != resolvedName {
		// Different function entirely: the join landed on the wrong call.
		return DisagreeUnknown
	}
	if recordedOwner == "" || resolvedOwner == "" {
		return DisagreeUnknown
	}

	if facts.IsInterface != nil && facts.IsInterface(recordedOwner) {
		return DisagreeInterface
	}
	if facts.Embeds != nil && facts.Embeds(recordedOwner, resolvedOwner) {
		return DisagreePromoted
	}
	return DisagreeUnknown
}

// splitOwner splits a function ID into the type or package that owns it and the
// bare function name: `pkg/sub.Type.Method` -> (`pkg/sub.Type`, `Method`).
func splitOwner(id string) (owner, name string) {
	i := strings.LastIndexByte(id, '.')
	if i < 0 {
		return "", id
	}
	return id[:i], id[i+1:]
}
