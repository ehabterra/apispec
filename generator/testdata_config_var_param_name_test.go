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

package generator

import (
	"strings"
	"testing"

	"github.com/ehabterra/apispec/internal/spec"
)

// A parameter's name is what the CLIENT sends, so it has to be a value the
// source settles. A package-level var initialised by a CALL does not settle
// one — the deployment decides it — and rendering that initializer emitted a
// header name no client could ever send:
//
//	name: func(s string) …/setting.k(REVERSE_PROXY_AUTHENTICATION_USER).MustString(X-WEBAUTH-USER)
//
// the same class of defect as rendering an argument as a path (#461). On the
// real project the expression rendered EMPTY instead, producing a parameter
// with no name at all, which is where issue #452 started.
func TestTestdata_ConfigVarParamName(t *testing.T) {
	out := loadTestdata(t, "config_var_param_name", spec.DefaultHTTPConfig())
	noDanglingRefs(t, out)

	item, ok := out.Paths["/whoami"]
	if !ok {
		t.Fatalf("/whoami missing; have %v", mapPathKeys(out.Paths))
	}
	op := opFor(item, "GET")
	if op == nil {
		t.Fatal("/whoami: no GET operation")
	}

	var names []string
	for _, p := range op.Parameters {
		names = append(names, p.Name)

		// Every parameter must be sendable: named, and not a rendered
		// expression. Checked on the SHAPE rather than on the one bad string,
		// so any other expression leaking into a name is caught too.
		if p.Ref == "" && p.Name == "" {
			t.Error("a parameter with no name — invalid OpenAPI, and unmatchable by any client")
		}
		for _, tell := range []string{"(", ")", "func", " + ", "/"} {
			if strings.Contains(p.Name, tell) {
				t.Errorf("parameter name %q carries %q — that is a rendered Go expression, "+
					"not a header a client can send", p.Name, tell)
			}
		}
	}

	// The names the source DOES settle must survive: declining the unknowable
	// one must not cost the knowable ones.
	for _, want := range []string{"X-Fixed", "X-Literal"} {
		found := false
		for _, n := range names {
			if n == want {
				found = true
			}
		}
		if !found {
			t.Errorf("parameter %q is missing; have %v — a var holding a literal and a literal "+
				"argument are both resolvable", want, names)
		}
	}
	if len(names) != 2 {
		t.Errorf("parameters = %v, want exactly the two resolvable ones — the config-var header "+
			"is not knowable from the source and must contribute nothing", names)
	}
}
