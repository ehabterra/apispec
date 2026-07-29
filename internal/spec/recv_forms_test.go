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
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// recvFormsFixture builds a metadata Call with a recorded receiver and,
// optionally, the receiver it was written against.
func recvFormsFixture(t *testing.T, pkg, recorded, written string) (*metadata.Metadata, *metadata.Call) {
	t.Helper()
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	call := &metadata.Call{
		Meta:     meta,
		Pkg:      meta.StringPool.Get(pkg),
		RecvType: meta.StringPool.Get(recorded),
	}
	call.SetWrittenRecvType(meta.StringPool.Get(written))
	return meta, call
}

// TestRecvFormsKeepsBothFactsAndNeitherMore pins the shape of the answer: the
// recorded receiver first (so a caller wanting one answer takes forms[0]), the
// written-against one only when it exists and differs.
func TestRecvFormsKeepsBothFactsAndNeitherMore(t *testing.T) {
	cases := []struct {
		name                   string
		pkg, recorded, written string
		want                   []string
		why                    string
	}{
		{
			name: "legacy call has one form",
			pkg:  "net/http", recorded: "*Request", written: "",
			want: []string{"net/http.*Request"},
			why:  "nothing sets the written receiver without a resolved call graph",
		},
		{
			name: "resolved past an interface keeps both",
			pkg:  "example.com/app", recorded: "*recorder", written: "ResponseWriter",
			want: []string{"example.com/app.*recorder", "example.com/app.ResponseWriter"},
			why:  "the concrete type runs and the interface is what a pattern names",
		},
		{
			name: "identical written receiver is not repeated",
			pkg:  "net/http", recorded: "Header", written: "Header",
			want: []string{"net/http.Header"},
			why:  "a duplicate form would make every regex test the same string twice",
		},
		{
			name: "a receiverless call qualifies to its package",
			pkg:  "net/http", recorded: "", written: "",
			want: []string{"net/http"},
			why:  "pre-existing fqOwner semantics: a plain function's owner IS its package, and the matchers scope on that",
		},
		{
			name: "nothing to qualify at all",
			pkg:  "", recorded: "", written: "",
			want: nil,
			why:  "no package and no receiver leaves no owner to match",
		},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			meta, call := recvFormsFixture(t, tt.pkg, tt.recorded, tt.written)
			got := recvForms(NewContextProvider(meta), call)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("recvForms = %v, want %v — %s", got, tt.want, tt.why)
			}
		})
	}
}

// TestMatchesRecvTypeAcceptsEitherForm is the rule issue #260 turns on: a
// pattern scoped to an interface must keep matching a call whose graph has been
// resolved past that interface.
func TestMatchesRecvTypeAcceptsEitherForm(t *testing.T) {
	meta, call := recvFormsFixture(t, "example.com/app", "*recorder", "ResponseWriter")
	cp := NewContextProvider(meta)

	t.Run("exact match on the written interface", func(t *testing.T) {
		if !matchesRecvType(cp, call, "example.com/app.ResponseWriter", "") {
			t.Error("a pattern naming the interface stopped matching once the callee was resolved to the concrete type")
		}
	})
	t.Run("exact match on the concrete type", func(t *testing.T) {
		if !matchesRecvType(cp, call, "example.com/app.*recorder", "") {
			t.Error("a pattern naming the concrete type must still match — it is what actually runs")
		}
	})
	t.Run("regex match on either", func(t *testing.T) {
		if !matchesRecvType(cp, call, "", `\.ResponseWriter$`) {
			t.Error("a regex scoped to the interface stopped matching")
		}
		if !matchesRecvType(cp, call, "", `\.\*recorder$`) {
			t.Error("a regex scoped to the concrete type stopped matching")
		}
	})
	t.Run("an unrelated scope still rejects", func(t *testing.T) {
		if matchesRecvType(cp, call, "net/url.Values", "") {
			t.Error("accepting either form must not accept every form")
		}
		if matchesRecvType(cp, call, "", `^net/url\.`) {
			t.Error("accepting either form must not accept every form")
		}
	})
	t.Run("an unscoped pattern matches anything", func(t *testing.T) {
		if !matchesRecvType(cp, call, "", "") {
			t.Error("a pattern that scopes no receiver must not be narrowed by this")
		}
	})
	t.Run("a scoped pattern rejects a receiverless call", func(t *testing.T) {
		_, plain := recvFormsFixture(t, "example.com/app", "", "")
		if matchesRecvType(cp, plain, "example.com/app.Router", "") {
			t.Error("a plain function matched a receiver-scoped pattern")
		}
	})
	t.Run("an invalid regex rejects rather than panics", func(t *testing.T) {
		if matchesRecvType(cp, call, "", "([") {
			t.Error("an unparseable regex must not match")
		}
	})
}

// TestWrittenRecvTypeEncodingKeepsUnsetAtZero pins the encoding that keeps every
// existing metadata golden byte-identical: unset must be the Go zero value, so
// `omitempty` drops the field, because the pool's "no string" is -1 and -1 would
// serialise on every call in the project.
func TestWrittenRecvTypeEncodingKeepsUnsetAtZero(t *testing.T) {
	var call metadata.Call
	if call.WrittenRecv != 0 {
		t.Fatalf("zero Call has WrittenRecv=%d; omitempty would serialise it", call.WrittenRecv)
	}
	if got := call.WrittenRecvType(); got != -1 {
		t.Errorf("unset WrittenRecvType() = %d, want -1 (the pool's no-string)", got)
	}

	// Pool index 0 is a real index and must survive the round trip — it is the
	// case a naive `omitempty` on the raw index would silently drop.
	call.SetWrittenRecvType(0)
	if call.WrittenRecv != 1 {
		t.Errorf("SetWrittenRecvType(0) stored %d, want 1", call.WrittenRecv)
	}
	if got := call.WrittenRecvType(); got != 0 {
		t.Errorf("WrittenRecvType() = %d after setting index 0", got)
	}

	call.SetWrittenRecvType(-1)
	if call.WrittenRecv != 0 || call.WrittenRecvType() != -1 {
		t.Error("clearing with -1 did not return the field to unset")
	}
}
