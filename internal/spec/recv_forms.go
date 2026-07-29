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

import "github.com/ehabterra/apispec/internal/metadata"

// recvForms returns every fully-qualified receiver a call may legitimately be
// matched on: the one recorded, and — when a resolved call graph replaced it —
// the one the call was WRITTEN against.
//
// Both are facts about the same call and neither subsumes the other (golden rule
// #7). The concrete type is what actually runs, so a pattern scoped to it should
// match; the written type is what the source says, so a pattern scoped to THAT
// should match too. A pattern config names interfaces — `net/http.ResponseWriter`,
// `net/http.Header`, `net/url.Values` — precisely because that is what a reader
// of the code sees, and resolving the graph replaces exactly those names. Matching
// only the recorded form makes every interface-scoped pattern stop matching, and
// every interface-scoped EXCLUSION stop excluding, as the call graph gets better
// (issue #260).
//
// The recorded form is always first, so a caller that wants the primary answer
// can take forms[0]. The written form is omitted when unset or identical, which
// is every call on a legacy run.
func recvForms(cp ContextProvider, call *metadata.Call) []string {
	if cp == nil || call == nil {
		return nil
	}
	recorded := fqOwner(cp, call.Pkg, call.RecvType)
	written := call.WrittenRecvType()
	if written < 0 {
		if recorded == "" {
			return nil
		}
		return []string{recorded}
	}
	writtenFq := qualifyOwner(cp.GetString(call.Pkg), cp.GetString(written))
	switch {
	case recorded == "":
		if writtenFq == "" {
			return nil
		}
		return []string{writtenFq}
	case writtenFq == "" || writtenFq == recorded:
		return []string{recorded}
	default:
		return []string{recorded, writtenFq}
	}
}

// matchesRecvType applies a pattern's receiver scope to a call, accepting any of
// the call's receiver forms.
//
// The regex takes precedence over the exact name, matching the order every
// pattern matcher already used. An empty pattern scopes nothing and matches.
func matchesRecvType(cp ContextProvider, call *metadata.Call, exact, regex string) bool {
	if regex == "" && exact == "" {
		return true
	}
	forms := recvForms(cp, call)
	if len(forms) == 0 {
		// No receiver at all: a scoped pattern cannot match a plain function.
		forms = []string{""}
	}
	if regex != "" {
		re, err := cachedRegex(regex)
		if err != nil {
			return false
		}
		for _, form := range forms {
			if re.MatchString(form) {
				return true
			}
		}
		return false
	}
	for _, form := range forms {
		if exact == form {
			return true
		}
	}
	return false
}
