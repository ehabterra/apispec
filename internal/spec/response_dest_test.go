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

// TestResponseDestResolver_Disabled: with no ResponseContext configured the
// resolver is permissive, preserving prior behaviour (zero-drift guarantee).
func TestResponseDestResolver_Disabled(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{}, cp)
	if r.Enabled() {
		t.Fatal("expected resolver to be disabled with no WriterTypeRegexes")
	}
	// Even a clearly-non-writer destination is accepted when disabled.
	if !r.IsResponseDest(mkIdent(meta, "buf", "*bytes.Buffer"), &metadata.CallGraphEdge{}) {
		t.Fatal("disabled resolver must be permissive")
	}
}

// TestResponseDestResolver_WriterTypes: once ResponseContext is configured, a
// bare writer-typed destination qualifies and a non-writer destination does not.
func TestResponseDestResolver_WriterTypes(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{
		Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext},
	}, cp)
	if !r.Enabled() {
		t.Fatal("resolver should be enabled with netHTTPResponseContext")
	}

	cases := []struct {
		name string
		arg  *metadata.CallArgument
		want bool
	}{
		{"response writer param w", mkIdent(meta, "w", "net/http.ResponseWriter"), true},
		{"httptest recorder", mkIdent(meta, "rec", "*net/http/httptest.ResponseRecorder"), true},
		{"bytes.Buffer is not a writer", mkIdent(meta, "buf", "*bytes.Buffer"), false},
		{"hash is not a writer", mkIdent(meta, "h", "hash.Hash"), false},
		{"untyped ident is not a writer", mkIdent(meta, "x", ""), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := r.IsResponseDest(tc.arg, &metadata.CallGraphEdge{})
			if got != tc.want {
				t.Errorf("IsResponseDest(%s) = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestResponseDestResolver_AddressOf: &w traces the same as w (address-of is
// stripped), matching how json.NewEncoder(&buf) / (w) appear.
func TestResponseDestResolver_AddressOf(t *testing.T) {
	meta := newTestMeta()
	cp := NewContextProvider(meta)
	r := newResponseDestResolver(&APISpecConfig{
		Framework: FrameworkConfig{ResponseContext: netHTTPResponseContext},
	}, cp)

	addr := metadata.NewCallArgument(meta)
	addr.SetKind(metadata.KindUnary)
	addr.X = mkIdent(meta, "w", "net/http.ResponseWriter")
	if !r.IsResponseDest(addr, &metadata.CallGraphEdge{}) {
		t.Error("&w should trace to the response writer")
	}

	addrBuf := metadata.NewCallArgument(meta)
	addrBuf.SetKind(metadata.KindUnary)
	addrBuf.X = mkIdent(meta, "buf", "*bytes.Buffer")
	if r.IsResponseDest(addrBuf, &metadata.CallGraphEdge{}) {
		t.Error("&buf must not trace to the response writer")
	}
}
