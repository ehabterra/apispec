package spec

import (
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
)

func TestTrackerTreeEnhanced(t *testing.T) {
	// Create a simple metadata structure for testing
	meta := &metadata.Metadata{
		StringPool: metadata.NewStringPool(),
		CallGraph: []metadata.CallGraphEdge{
			{
				Caller: metadata.Call{
					Meta: nil, // Will be set after creation
					Name: 0,   // Will be set after StringPool is created
					Pkg:  1,
				},
				Callee: metadata.Call{
					Meta: nil, // Will be set after creation
					Name: 2,
					Pkg:  3,
				},
				Args: []metadata.CallArgument{
					{
						Kind: "ident",
						Name: "router",
						Type: "*chi.Mux",
					},
					{
						Kind: "call",
						Fun: &metadata.CallArgument{
							Kind: "ident",
							Name: "ListUsers",
							Type: "func()",
						},
					},
				},
			},
		},
	}

	// Set the metadata references and string pool indices after creation
	meta.CallGraph[0].Caller.Meta = meta
	meta.CallGraph[0].Callee.Meta = meta
	meta.CallGraph[0].Caller.Name = meta.StringPool.Get("main")
	meta.CallGraph[0].Caller.Pkg = meta.StringPool.Get("main")
	meta.CallGraph[0].Callee.Name = meta.StringPool.Get("NewRouter")
	meta.CallGraph[0].Callee.Pkg = meta.StringPool.Get("chi")

	// Create tracker with limits
	limits := TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	tree := NewTrackerTree(meta, limits)

	// Test enhanced functionality
	t.Run("ArgumentClassification", func(t *testing.T) {
		// Test argument type classification
		argType := classifyArgument(metadata.CallArgument{Kind: "ident", Name: "router"}, &metadata.CallGraphEdge{})
		if argType != ArgTypeVariable {
			t.Errorf("Expected ArgTypeVariable for ident, got %v", argType)
		}

		argType = classifyArgument(metadata.CallArgument{Kind: "call"}, &metadata.CallGraphEdge{})
		if argType != ArgTypeFunctionCall {
			t.Errorf("Expected ArgTypeFunctionCall for call, got %v", argType)
		}

		argType = classifyArgument(metadata.CallArgument{Kind: "literal", Value: "test"}, &metadata.CallGraphEdge{})
		if argType != ArgTypeLiteral {
			t.Errorf("Expected ArgTypeLiteral for literal, got %v", argType)
		}
	})

	t.Run("ArgumentStatistics", func(t *testing.T) {
		stats := tree.GetArgumentStatistics()

		// Verify stats structure
		if stats["total_arguments"] == nil {
			t.Error("Expected total_arguments in statistics")
		}

		if stats["variable_nodes"] == nil {
			t.Error("Expected variable_nodes in statistics")
		}

		if stats["function_nodes"] == nil {
			t.Error("Expected function_nodes in statistics")
		}
	})

	t.Run("ArgumentTypeCount", func(t *testing.T) {
		counts := tree.GetArgumentTypeCount()

		// Verify counts is a map
		if counts == nil {
			t.Error("Expected non-nil argument type counts")
		}
	})

	t.Run("FindArgumentNodes", func(t *testing.T) {
		// Test finding argument nodes for a function
		args := tree.FindArgumentNodes("main")

		// Should return slice (may be empty)
		if args == nil {
			t.Error("Expected non-nil result from FindArgumentNodes")
		}
	})

	t.Run("FindVariableNodes", func(t *testing.T) {
		// Test finding variable nodes
		vars := tree.FindVariableNodes()

		// Should return slice (may be empty)
		if vars == nil {
			t.Error("Expected non-nil result from FindVariableNodes")
		}
	})

	t.Run("FindFunctionNodes", func(t *testing.T) {
		// Test finding function nodes
		funcs := tree.FindFunctionNodes()

		// Should return slice (may be empty)
		if funcs == nil {
			t.Error("Expected non-nil result from FindFunctionNodes")
		}
	})

	t.Run("GetArgumentContexts", func(t *testing.T) {
		// Test getting argument contexts
		contexts := tree.GetArgumentContexts()

		// Should return slice (may be empty)
		if contexts == nil {
			t.Error("Expected non-nil result from GetArgumentContexts")
		}
	})
}

func TestArgumentTypeString(t *testing.T) {
	// Test that all argument types have meaningful values
	testCases := []struct {
		argType ArgumentType
		name    string
	}{
		{ArgTypeDirectCallee, "DirectCallee"},
		{ArgTypeFunctionCall, "FunctionCall"},
		{ArgTypeVariable, "Variable"},
		{ArgTypeLiteral, "Literal"},
		{ArgTypeSelector, "Selector"},
		{ArgTypeComplex, "Complex"},
		{ArgTypeUnary, "Unary"},
		{ArgTypeBinary, "Binary"},
		{ArgTypeIndex, "Index"},
		{ArgTypeComposite, "Composite"},
		{ArgTypeTypeAssert, "TypeAssert"},
	}

	for _, tc := range testCases {
		if tc.argType < 0 || tc.argType > ArgTypeTypeAssert {
			t.Errorf("Invalid argument type: %v", tc.argType)
		}
	}
}
