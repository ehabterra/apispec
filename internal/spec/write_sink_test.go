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

// writeSinkMatcher builds a matcher whose ResponseContext configures the JSON
// body transforms the write-sink resolver traces through.
func writeSinkMatcher(meta *metadata.Metadata) *ResponsePatternMatcherImpl {
	cfg := &APISpecConfig{Defaults: Defaults{ResponseContentType: "application/json"}}
	cfg.Framework.ResponseContext.BodyTransforms = []BodyTransform{
		{CallRegex: `^Marshal$`, PkgRegex: `^encoding/json$`, ArgIndex: 0},
		{CallRegex: `^Encode$`, ArgIndex: 0}, // empty PkgRegex → any package
	}
	return &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			cfg:             cfg,
			contextProvider: NewContextProvider(meta),
			schemaMapper:    NewSchemaMapper(cfg),
		},
	}
}

// TestMatchBodyTransform pins the transform matcher: callee+package matching,
// empty-PkgRegex-matches-any, and non-transform/empty rejection.
func TestMatchBodyTransform(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := writeSinkMatcher(meta)

	if idx, ok := m.matchBodyTransform("Marshal", "encoding/json"); !ok || idx != 0 {
		t.Errorf("Marshal/encoding/json: got (%d,%v), want (0,true)", idx, ok)
	}
	// Empty PkgRegex matches any package.
	if _, ok := m.matchBodyTransform("Encode", "some/other/pkg"); !ok {
		t.Error("Encode with any pkg should match (empty PkgRegex)")
	}
	// Wrong package for Marshal → no match.
	if _, ok := m.matchBodyTransform("Marshal", "gopkg.in/yaml.v3"); ok {
		t.Error("Marshal in a non-json package should not match")
	}
	// Non-transform callee → no match.
	if _, ok := m.matchBodyTransform("Sprintf", "fmt"); ok {
		t.Error("Sprintf should not match")
	}
	// Empty callee → no match.
	if _, ok := m.matchBodyTransform("", "encoding/json"); ok {
		t.Error("empty callee should not match")
	}
}

// marshalHelper builds `func h(<params>) []byte { b, _ := json.Marshal(payload); return b }`
// as a Function, where payload is the parameter named payloadParam. Used to
// exercise helperSerializedParam / paramIndexOf without a full metadata graph.
func marshalHelper(meta *metadata.Metadata, params []string, payloadParam string) *metadata.Function {
	sig := metadata.NewCallArgument(meta)
	sig.SetKind(metadata.KindFuncType)
	for _, p := range params {
		sig.Args = append(sig.Args, identArg(meta, p))
	}
	marshalCall := metadata.NewCallArgument(meta)
	marshalCall.SetKind(metadata.KindCall)
	marshalCall.Args = []*metadata.CallArgument{identArg(meta, payloadParam)}
	return &metadata.Function{
		Signature: *sig,
		Returns:   [][]metadata.CallArgument{{*identArg(meta, "b")}},
		AssignmentMap: map[string][]metadata.Assignment{
			"b": {{CalleeFunc: "Marshal", CalleePkg: "encoding/json", Value: *marshalCall}},
		},
	}
}

// TestHelperSerializedParam pins the returned-local transform case: which
// parameter a helper serializes and returns, its positional index (including a
// non-zero index), an unknown-name lookup, and the raw-bytes (no-transform) residue.
func TestHelperSerializedParam(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := writeSinkMatcher(meta)

	// Single param: `b, _ := json.Marshal(v); return b` → serialized param "v".
	fn := marshalHelper(meta, []string{"v"}, "v")
	if got := m.helperSerializedParam(fn); got != "v" {
		t.Errorf("helperSerializedParam single: got %q, want v", got)
	}
	if i := paramIndexOf(fn, "v"); i != 0 {
		t.Errorf("paramIndexOf v: got %d, want 0", i)
	}

	// Multi-param, payload is the second: index 1.
	fn2 := marshalHelper(meta, []string{"prefix", "v"}, "v")
	if got := m.helperSerializedParam(fn2); got != "v" {
		t.Errorf("helperSerializedParam multi: got %q, want v", got)
	}
	if i := paramIndexOf(fn2, "v"); i != 1 {
		t.Errorf("paramIndexOf v (multi): got %d, want 1", i)
	}

	// Unknown param name → -1.
	if i := paramIndexOf(fn2, "missing"); i != -1 {
		t.Errorf("paramIndexOf missing: got %d, want -1", i)
	}

	// Helper that returns raw bytes (no transform) → no serialized param.
	raw := &metadata.Function{
		Returns: [][]metadata.CallArgument{{*identArg(meta, "b")}},
		// b has no transform assignment recorded.
	}
	if got := m.helperSerializedParam(raw); got != "" {
		t.Errorf("helperSerializedParam raw: got %q, want empty", got)
	}
}

// TestHelperSerializedParam_InlineReturn pins the `return json.Marshal(p)`
// branch (the returned value is itself the transform call) and the
// literal-payload residue.
func TestHelperSerializedParam_InlineReturn(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := writeSinkMatcher(meta)

	// func h(p T) []byte { return json.Marshal(p) } — the return value is itself
	// the transform call (KindCall branch).
	marshalFun := identArg(meta, "Marshal")
	marshalFun.SetPkg("encoding/json")
	marshalCall := metadata.NewCallArgument(meta)
	marshalCall.SetKind(metadata.KindCall)
	marshalCall.Fun = marshalFun
	marshalCall.Args = []*metadata.CallArgument{identArg(meta, "p")}
	fn := &metadata.Function{
		Signature: metadata.CallArgument{},
		Returns:   [][]metadata.CallArgument{{*marshalCall}},
	}
	if got := m.helperSerializedParam(fn); got != "p" {
		t.Errorf("inline-return: got %q, want p", got)
	}

	// A returned transform call whose payload is a literal (not an ident) yields
	// no serialized parameter.
	litCall := metadata.NewCallArgument(meta)
	litCall.SetKind(metadata.KindCall)
	lf := identArg(meta, "Marshal")
	lf.SetPkg("encoding/json")
	litCall.Fun = lf
	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	litCall.Args = []*metadata.CallArgument{lit}
	fnLit := &metadata.Function{ReturnVars: []metadata.CallArgument{*litCall}}
	if got := m.helperSerializedParam(fnLit); got != "" {
		t.Errorf("literal payload: got %q, want empty", got)
	}
}

// TestUnwrapWriteSink_ParenStrip pins the address-of/deref/paren strip loop and
// the fully-stripped-to-nil path.
func TestUnwrapWriteSink_ParenStrip(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := writeSinkMatcher(meta)
	edge := &metadata.CallGraphEdge{}

	// *(b) — parenthesized deref wrapping an ident exercises the strip loop.
	inner := identArg(meta, "b")
	paren := metadata.NewCallArgument(meta)
	paren.SetKind(metadata.KindParen)
	paren.X = inner
	star := metadata.NewCallArgument(meta)
	star.SetKind(metadata.KindStar)
	star.X = paren
	// No assignment for b at this edge → resolves to nil, but the strip runs.
	if got := m.unwrapWriteSink(star, edge); got != nil {
		t.Errorf("paren/star strip with no assignment: got %v, want nil", got)
	}

	// A fully-stripped nil (unary wrapping nothing) → nil.
	unary := metadata.NewCallArgument(meta)
	unary.SetKind(metadata.KindUnary)
	if got := m.unwrapWriteSink(unary, edge); got != nil {
		t.Errorf("unary wrapping nil: got %v, want nil", got)
	}
}

// TestUnwrapWriteSink_Guards pins the early returns: nil arg, nil edge, and no
// configured BodyTransforms.
func TestUnwrapWriteSink_Guards(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := writeSinkMatcher(meta)
	edge := &metadata.CallGraphEdge{}

	if got := m.unwrapWriteSink(nil, edge); got != nil {
		t.Error("nil arg should return nil")
	}
	if got := m.unwrapWriteSink(identArg(meta, "b"), nil); got != nil {
		t.Error("nil edge should return nil")
	}

	// No configured transforms → nil regardless of arg.
	empty := &ResponsePatternMatcherImpl{BasePatternMatcher: &BasePatternMatcher{
		cfg: &APISpecConfig{}, contextProvider: NewContextProvider(meta), schemaMapper: NewSchemaMapper(&APISpecConfig{}),
	}}
	if got := empty.unwrapWriteSink(identArg(meta, "b"), edge); got != nil {
		t.Error("no BodyTransforms should return nil")
	}
}

// TestUnwrapHelperReturn_Guards pins the guards: an unknown helper (not in
// metadata) and a nil callee function.
func TestUnwrapHelperReturn_Guards(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}
	m := writeSinkMatcher(meta)
	edge := &metadata.CallGraphEdge{}

	// A call to a helper that isn't in metadata (findFunctionByName → nil) → nil.
	call := metadata.NewCallArgument(meta)
	call.SetKind(metadata.KindCall)
	call.Fun = identArg(meta, "helper")
	if got := m.unwrapHelperReturn(call, edge); got != nil {
		t.Error("unknown helper should return nil")
	}
	// nil Fun → nil.
	bad := metadata.NewCallArgument(meta)
	bad.SetKind(metadata.KindCall)
	if got := m.unwrapHelperReturn(bad, edge); got != nil {
		t.Error("nil Fun should return nil")
	}
}
