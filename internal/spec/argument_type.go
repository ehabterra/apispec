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
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

// The pieces of the tracker that outlived the eager tree (issue #425): how a
// node came to be, the key an assignment is indexed by, a single call edge
// presented as a node, and one string-pool helper. They lived in tracker.go
// because that is where the eager tree needed them; the lazy tree needs them
// too, so they are here rather than gone.

// ArgumentType represents the classification of an argument.
//
// uint8, not int: it is a field of every tracker node, and the lazy tree
// materialises millions of those on a real service, where expansion is bound
// by the bytes it writes rather than by anything it computes. Eleven values
// need one byte, and one byte lets the node's flags share a single word.
type ArgumentType uint8

const (
	ArgTypeDirectCallee ArgumentType = iota // Direct function call (existing callee)
	ArgTypeFunctionCall                     // Function call as argument
	ArgTypeVariable                         // Variable reference
	ArgTypeLiteral                          // Literal value
	ArgTypeSelector                         // Field/method selector
	ArgTypeComplex                          // Complex expression
	ArgTypeUnary                            // Unary expression (*ptr, &val)
	ArgTypeBinary                           // Binary expression (a + b)
	ArgTypeIndex                            // Index expression (arr[i])
	ArgTypeComposite                        // Composite literal (struct{})
	ArgTypeTypeAssert                       // Type assertion (val.(type))
)

// String returns the string representation of ArgumentType
func (at ArgumentType) String() string {
	switch at {
	case ArgTypeDirectCallee:
		return "DirectCallee"
	case ArgTypeFunctionCall:
		return "FunctionCall"
	case ArgTypeVariable:
		return "Variable"
	case ArgTypeLiteral:
		return "Literal"
	case ArgTypeSelector:
		return "Selector"
	case ArgTypeComplex:
		return "Complex"
	case ArgTypeUnary:
		return "Unary"
	case ArgTypeBinary:
		return "Binary"
	case ArgTypeIndex:
		return "Index"
	case ArgTypeComposite:
		return "Composite"
	case ArgTypeTypeAssert:
		return "TypeAssert"
	default:
		return "Unknown"
	}
}

// edgeNode presents a single call edge as a TrackerNodeInterface, for the
// matchers that ask "would this pattern match this call?" without a tree to ask
// it of — wrapper derivation scans the call graph directly.
//
// This is what the eager tree's node was borrowed for before it was removed
// (issue #425). A dedicated three-line adapter says what is actually needed:
// no key, no children, no parent, no type parameters — just the edge.
type edgeNode struct {
	edge *metadata.CallGraphEdge
}

func newEdgeNode(edge *metadata.CallGraphEdge) *edgeNode { return &edgeNode{edge: edge} }

func (n *edgeNode) GetKey() string                      { return "" }
func (n *edgeNode) GetParent() TrackerNodeInterface     { return nil }
func (n *edgeNode) GetChildren() []TrackerNodeInterface { return nil }
func (n *edgeNode) GetEdge() *metadata.CallGraphEdge    { return n.edge }
func (n *edgeNode) GetArgument() *metadata.CallArgument { return nil }
func (n *edgeNode) GetTypeParamMap() map[string]string  { return nil }

// assignmentKey identifies one assignment for the producer index: which
// variable, in which package, holding which concrete type, inside which
// container.
//
// The composition is load-bearing rather than incidental — the lazy tree's
// producer links are looked up by exactly this key, so the fields and their
// order in String() are what makes a group assigned in one function findable
// from the call that mounts it (issue #275).
type assignmentKey struct {
	Name      string
	Pkg       string
	Type      string
	Container string
}

func (k assignmentKey) String() string {
	return k.Pkg + k.Type + k.Name + k.Container
}

// getString retrieves a string value from the metadata string pool.
func getString(meta *metadata.Metadata, index int) string {
	if meta == nil || meta.StringPool == nil {
		return ""
	}
	return meta.StringPool.GetString(index)
}

// classifyArgument determines the type of an argument for enhanced processing
func classifyArgument(arg *metadata.CallArgument) ArgumentType {
	switch arg.GetKind() {
	case metadata.KindCall, metadata.KindFuncLit:
		return ArgTypeFunctionCall
	case metadata.KindIdent:
		if strings.HasPrefix(arg.GetType(), "func(") {
			return ArgTypeFunctionCall
		}
		return ArgTypeVariable
	case metadata.KindLiteral:
		return ArgTypeLiteral
	case metadata.KindSelector:
		return ArgTypeSelector
	case metadata.KindUnary:
		return ArgTypeUnary
	case metadata.KindBinary:
		return ArgTypeBinary
	case metadata.KindIndex:
		return ArgTypeIndex
	case metadata.KindCompositeLit:
		return ArgTypeComposite
	case metadata.KindTypeAssert:
		return ArgTypeTypeAssert
	default:
		return ArgTypeComplex
	}
}
