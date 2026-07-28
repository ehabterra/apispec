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

import "testing"

// TestClassify pins which disagreements may be acted on. Measured over gitea,
// the four classes cover 10,182 real disagreements: 2,880 interface calls,
// 4,492 promoted methods, 1,570 ambiguous sites and 1,240 unexplained.
func TestClassify(t *testing.T) {
	facts := TypeFacts{
		IsInterface: func(q string) bool { return q == "app.Storage" },
		Embeds: func(outer, inner string) bool {
			return outer == "app.Context" && inner == "app.Base"
		},
	}

	tests := []struct {
		name     string
		recorded string
		resolved []string
		want     Disagreement
		why      string
	}{
		{
			name:     "interface method resolved to its implementation",
			recorded: "app.Storage.List",
			resolved: []string{"app.s3driver.List"},
			want:     DisagreeInterface,
			why:      "the recorded owner is an interface — this is what the resolved graph is for",
		},
		{
			name:     "promoted method resolved to the declaring type",
			recorded: "app.Context.FormString",
			resolved: []string{"app.Base.FormString"},
			want:     DisagreePromoted,
			why:      "Context embeds Base, so Go promotes the method and both names describe one function",
		},
		{
			name:     "several implementations stay ambiguous",
			recorded: "app.Storage.List",
			resolved: []string{"app.s3driver.List", "app.localDriver.List"},
			want:     DisagreeAmbiguous,
			why:      "picking one would invent a concrete type the program may never use (golden rule #7)",
		},
		{
			name:     "a different function entirely is a mis-join",
			recorded: "app.Storage.List",
			resolved: []string{"app.s3driver.Save"},
			want:     DisagreeUnknown,
			why:      "the names differ, so the join landed on the wrong call and must not be acted on",
		},
		{
			name:     "unrelated owner with the same method name",
			recorded: "app.Widget.Close",
			resolved: []string{"other.File.Close"},
			want:     DisagreeUnknown,
			why:      "neither an interface nor an embedding relation explains it",
		},
		{
			name:     "no resolved target",
			recorded: "app.Storage.List",
			resolved: nil,
			want:     DisagreeUnknown,
		},
		{
			name:     "package-level function has no owner type",
			recorded: "app.Helper",
			resolved: []string{"other.Helper"},
			want:     DisagreeUnknown,
			why:      "a package is not a type; no interface or embedding relation can hold",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Classify(tt.recorded, tt.resolved, facts); got != tt.want {
				t.Errorf("Classify(%q, %v) = %v, want %v — %s", tt.recorded, tt.resolved, got, tt.want, tt.why)
			}
		})
	}
}

// TestClassifyWithoutFacts pins that a caller supplying no type facts gets no
// confident answers rather than wrong ones.
func TestClassifyWithoutFacts(t *testing.T) {
	if got := Classify("app.Storage.List", []string{"app.s3driver.List"}, TypeFacts{}); got != DisagreeUnknown {
		t.Errorf("Classify with no facts = %v, want %v", got, DisagreeUnknown)
	}
}

func TestDisagreementString(t *testing.T) {
	for class, want := range map[Disagreement]string{
		DisagreeInterface: "interface->concrete",
		DisagreePromoted:  "promoted-through-embedding",
		DisagreeAmbiguous: "ambiguous",
		DisagreeUnknown:   "unexplained",
		Disagreement(99):  "unexplained",
	} {
		if got := class.String(); got != want {
			t.Errorf("Disagreement(%d).String() = %q, want %q", class, got, want)
		}
	}
}

func TestSplitOwner(t *testing.T) {
	for id, want := range map[string][2]string{
		"pkg/sub.Type.Method": {"pkg/sub.Type", "Method"},
		"pkg.Func":            {"pkg", "Func"},
		"Bare":                {"", "Bare"},
		"":                    {"", ""},
	} {
		owner, name := splitOwner(id)
		if owner != want[0] || name != want[1] {
			t.Errorf("splitOwner(%q) = (%q, %q), want (%q, %q)", id, owner, name, want[0], want[1])
		}
	}
}
