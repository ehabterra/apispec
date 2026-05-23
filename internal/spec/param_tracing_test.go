package spec

import (
	"testing"

	"github.com/ehabterra/apispec/internal/metadata"
)

// fakeNode is a minimal TrackerNodeInterface tailored for traceArgViaParent:
// only GetParent and GetEdge are exercised by the helper, so the rest of
// the interface returns zero values. This keeps the test free of the
// real TrackerTree's setup cost.
type fakeNode struct {
	parent TrackerNodeInterface
	edge   *metadata.CallGraphEdge
}

func (f *fakeNode) GetKey() string                                         { return "" }
func (f *fakeNode) GetParent() TrackerNodeInterface                        { return f.parent }
func (f *fakeNode) GetChildren() []TrackerNodeInterface                    { return nil }
func (f *fakeNode) GetEdge() *metadata.CallGraphEdge                       { return f.edge }
func (f *fakeNode) GetCallGraphEdge() *metadata.CallGraphEdge              { return f.edge }
func (f *fakeNode) GetCallArgument() *metadata.CallArgument                { return nil }
func (f *fakeNode) GetArgContext() string                                  { return "" }
func (f *fakeNode) GetArgIndex() int                                       { return 0 }
func (f *fakeNode) GetArgType() metadata.ArgumentType                      { return metadata.ArgTypeDirectCallee }
func (f *fakeNode) GetArgument() *metadata.CallArgument                    { return nil }
func (f *fakeNode) GetTypeParamMap() map[string]string                     { return nil }
func (f *fakeNode) GetRootAssignmentMap() map[string][]metadata.Assignment { return nil }

// TestTraceArgViaParent_ResolvesParameterToCallerArg mirrors the writeJSON
// case from issue: an inner call like WriteHeader(status) inside
// writeJSON(w, status, v) couldn't see the caller's literal because the
// matched call's arg is the parameter ident "status". The helper walks up
// to the parent (writeJSON's call site) and reads ParamArgMap["status"].
func TestTraceArgViaParent_ResolvesParameterToCallerArg(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	// Build the caller-site arg: http.StatusOK rendered as a selector.
	statusValue := metadata.NewCallArgument(meta)
	statusValue.SetKind(metadata.KindSelector)
	x := metadata.NewCallArgument(meta)
	x.SetKind(metadata.KindIdent)
	x.SetName("http")
	sel := metadata.NewCallArgument(meta)
	sel.SetKind(metadata.KindIdent)
	sel.SetName("StatusOK")
	statusValue.X = x
	statusValue.Sel = sel

	// Parent edge represents the call writeJSON(w, http.StatusOK, out)
	// from the route handler. ParamArgMap maps callee parameter names to
	// the actual call-site arguments.
	parentEdge := &metadata.CallGraphEdge{
		ParamArgMap: map[string]metadata.CallArgument{
			"status": *statusValue,
		},
	}
	parent := &fakeNode{edge: parentEdge}

	// Child node represents WriteHeader(status) inside writeJSON. Its arg
	// is the ident "status" — a parameter, not a literal.
	statusIdent := metadata.NewCallArgument(meta)
	statusIdent.SetKind(metadata.KindIdent)
	statusIdent.SetName("status")
	child := &fakeNode{parent: parent}

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: NewContextProvider(meta),
		},
	}

	got := matcher.traceArgViaParent(statusIdent, child)
	if got == nil {
		t.Fatal("expected to recover caller-site arg via parent's ParamArgMap, got nil")
	}
	if got.Sel == nil || got.Sel.GetName() != "StatusOK" {
		t.Errorf("expected recovered arg to point at http.StatusOK selector, got %+v", got)
	}
}

// TestTraceArgViaParent_ReturnsNilForLiteral guards the early-out: a
// literal arg (e.g. WriteHeader(200) called directly) needs no tracing
// and the helper must not paper over its own preconditions.
func TestTraceArgViaParent_ReturnsNilForLiteral(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	lit := metadata.NewCallArgument(meta)
	lit.SetKind(metadata.KindLiteral)
	lit.SetValue("200")

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: NewContextProvider(meta),
		},
	}

	if got := matcher.traceArgViaParent(lit, &fakeNode{}); got != nil {
		t.Errorf("expected nil for literal arg, got %+v", got)
	}
}

// TestTraceArgViaParent_ReturnsNilWhenParamNotMapped covers the case
// where the ident isn't actually a parameter of the enclosing function
// — the helper must not invent a value.
func TestTraceArgViaParent_ReturnsNilWhenParamNotMapped(t *testing.T) {
	meta := &metadata.Metadata{StringPool: metadata.NewStringPool()}

	parent := &fakeNode{edge: &metadata.CallGraphEdge{
		ParamArgMap: map[string]metadata.CallArgument{
			// Maps a different name only — "status" should not resolve.
			"other": *metadata.NewCallArgument(meta),
		},
	}}
	child := &fakeNode{parent: parent}

	ident := metadata.NewCallArgument(meta)
	ident.SetKind(metadata.KindIdent)
	ident.SetName("status")

	matcher := &ResponsePatternMatcherImpl{
		BasePatternMatcher: &BasePatternMatcher{
			contextProvider: NewContextProvider(meta),
		},
	}

	if got := matcher.traceArgViaParent(ident, child); got != nil {
		t.Errorf("expected nil when param not in ParamArgMap, got %+v", got)
	}
}
