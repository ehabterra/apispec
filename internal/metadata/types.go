package metadata

import (
	"fmt"
	"maps"
	"strings"
)

const (
	kindIdent           = "ident"
	kindLiteral         = "literal"
	kindSelector        = "selector"
	kindCall            = "call"
	kindRaw             = "raw"
	kindString          = "string"
	kindInt             = "int"
	kindFloat64         = "float64"
	kindRune            = "rune"
	kindComplex128      = "complex128"
	kindFuncLit         = "func_lit"
	kindUnary           = "unary"
	kindBinary          = "binary"
	kindIndex           = "index"
	kindIndexList       = "index_list"
	kindStar            = "star"
	kindParen           = "paren"
	kindArrayType       = "array_type"
	kindSlice           = "slice"
	kindCompositeLit    = "composite_lit"
	kindKeyValue        = "key_value"
	kindTypeAssert      = "type_assert"
	kindChanType        = "chan_type"
	kindMapType         = "map_type"
	kindStructType      = "struct_type"
	kindInterfaceType   = "interface_type"
	kindInterfaceMethod = "interface_method"
	kindEmbed           = "embed"
	kindField           = "field"
	kindEllipsis        = "ellipsis"
	kindFuncType        = "func_type"
	kindFuncResults     = "func_results"
)

// StringPool for deduplicating strings across metadata
type StringPool struct {
	strings map[string]int
	values  []string
}

func NewStringPool() *StringPool {
	return &StringPool{
		strings: make(map[string]int),
		values:  make([]string, 0),
	}
}

func (sp *StringPool) Get(s string) int {
	if s == "" {
		return -1
	}

	if idx, exists := sp.strings[s]; exists {
		return idx
	}

	if sp.strings == nil {
		return -1
	}

	idx := len(sp.values)
	sp.strings[s] = idx
	sp.values = append(sp.values, s)
	return idx
}

func (sp *StringPool) GetString(idx int) string {
	if idx >= 0 && idx < len(sp.values) {
		return sp.values[idx]
	}
	return ""
}

// GetSize returns the number of unique strings in the pool
func (sp *StringPool) GetSize() int {
	return len(sp.values)
}

// MarshalYAML implements yaml.Marshaler interface
func (sp *StringPool) MarshalYAML() (interface{}, error) {
	return sp.values, nil
}

// UnmarshalYAML implements yaml.Unmarshaler interface
func (sp *StringPool) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var values []string
	if err := unmarshal(&values); err != nil {
		return err
	}

	sp.values = values
	sp.strings = make(map[string]int)
	for i, s := range values {
		sp.strings[s] = i
	}
	return nil
}

// Finalize cleans up the string pool by removing the lookup map
func (sp *StringPool) Finalize() {
	// sp.strings = nil
}

// Metadata represents the complete metadata for a Go codebase
type Metadata struct {
	StringPool *StringPool         `yaml:"string_pool,omitempty"`
	Packages   map[string]*Package `yaml:"packages,omitempty"`
	CallGraph  []CallGraphEdge     `yaml:"call_graph,omitempty"`

	roots   []*CallGraphEdge            `yaml:"roots"`
	Callers map[string][]*CallGraphEdge `yaml:"-"`
	Callees map[string][]*CallGraphEdge `yaml:"-"`
	Args    map[string][]*CallGraphEdge `yaml:"-"`
}

func (m *Metadata) TraverseCallerChildren(edge *CallGraphEdge, action func(edge, child *CallGraphEdge)) {
	calleeID := stripID(edge.Callee.ID())
	if children, ok := m.Callers[calleeID]; ok {
		for _, child := range children {
			action(edge, child)

			m.TraverseCallerChildren(child, action)
		}
	}
}

func (m *Metadata) CallGraphRoots() []*CallGraphEdge {
	if len(m.roots) > 0 {
		return m.roots
	}

	// Search for root functions
	for i := range m.CallGraph {
		edge := &m.CallGraph[i]

		callerID := edge.Caller.ID()

		var root = true

		if _, exists := m.Callees[callerID]; exists {
			root = false
		}
		if _, exists := m.Args[callerID]; exists {
			root = false
		}

		// Only select main function from root function to be the root
		// and construct the tree based on it
		if root {
			m.roots = append(m.roots, edge)
		}
	}

	return m.roots
}

// Package represents a Go package
type Package struct {
	Files map[string]*File `yaml:"files,omitempty"`
	Types map[string]*Type `yaml:"types,omitempty"`
}

// File represents a Go source file
type File struct {
	Types           map[string]*Type     `yaml:"types,omitempty"`
	Functions       map[string]*Function `yaml:"functions,omitempty"`
	Variables       map[string]*Variable `yaml:"variables,omitempty"`
	StructInstances []StructInstance     `yaml:"struct_instances,omitempty"`
	// Selectors       []Selector           `yaml:"selectors"`
	Imports map[int]int `yaml:"imports"` // alias -> path
}

// Type represents a Go type
type Type struct {
	Name          int      `yaml:"name,omitempty"`
	Kind          int      `yaml:"kind,omitempty"`
	Target        int      `yaml:"target,omitempty"`
	Implements    []int    `yaml:"implements,omitempty"`
	ImplementedBy []int    `yaml:"implemented_by,omitempty"`
	Embeds        []int    `yaml:"embeds,omitempty"`
	Fields        []Field  `yaml:"fields,omitempty"`
	Scope         int      `yaml:"scope,omitempty"`
	Methods       []Method `yaml:"methods,omitempty"`
	Comments      int      `yaml:"comments,omitempty"`
	Tags          []int    `yaml:"tags,omitempty"`
}

// Field represents a struct field
type Field struct {
	Name     int `yaml:"name,omitempty"`
	Type     int `yaml:"type,omitempty"`
	Tag      int `yaml:"tag,omitempty"`
	Scope    int `yaml:"scope,omitempty"`
	Comments int `yaml:"comments,omitempty"`
}

// Method represents a method
type Method struct {
	Name         int          `yaml:"name,omitempty"`
	Receiver     int          `yaml:"receiver,omitempty"`
	Signature    CallArgument `yaml:"signature,omitempty"`
	SignatureStr int          `yaml:"signature_str,omitempty"`
	Position     int          `yaml:"position,omitempty"`
	Scope        int          `yaml:"scope,omitempty"`
	Comments     int          `yaml:"comments,omitempty"`
	Tags         []int        `yaml:"tags,omitempty"`

	// map of variable name to all assignments (for alias/reassignment tracking)
	AssignmentMap map[string][]Assignment `yaml:"assignments,omitempty"`
}

// Function represents a function
type Function struct {
	Name      int          `yaml:"name,omitempty"`
	Signature CallArgument `yaml:"signature,omitempty"`
	Position  int          `yaml:"position,omitempty"`
	Scope     int          `yaml:"scope,omitempty"`
	Comments  int          `yaml:"comments,omitempty"`
	Tags      []int        `yaml:"tags,omitempty"`

	// Type parameter names for generics
	TypeParams []string `yaml:"type_params,omitempty"`

	// Return value origins for tracing through return values
	ReturnVars []CallArgument `yaml:"return_vars,omitempty"`

	// map of variable name to all assignments (for alias/reassignment tracking)
	AssignmentMap map[string][]Assignment `yaml:"assignments,omitempty"`
}

// Variable represents a variable
type Variable struct {
	Name     int `yaml:"name,omitempty"`
	Tok      int `yaml:"tok,omitempty"`
	Type     int `yaml:"type,omitempty"`
	Value    int `yaml:"value,omitempty"`
	Position int `yaml:"position,omitempty"`
	Comments int `yaml:"comments,omitempty"`
}

// Selector represents a selector expression
type Selector struct {
	Expr     CallArgument `yaml:"expr,omitempty"`
	Kind     int          `yaml:"kind,omitempty"`
	Position int          `yaml:"position,omitempty"`
}

// StructInstance represents a struct literal instance
type StructInstance struct {
	Type     int         `yaml:"type,omitempty"`
	Position int         `yaml:"position,omitempty"`
	Fields   map[int]int `yaml:"fields,omitempty"`
}

// Assignment represents a variable assignment
type Assignment struct {
	VariableName int          `yaml:"variable_name,omitempty"`
	Pkg          int          `yaml:"pkg,omitempty"`
	ConcreteType int          `yaml:"concrete_type,omitempty"`
	Position     int          `yaml:"position,omitempty"`
	Scope        int          `yaml:"scope,omitempty"`
	Value        CallArgument `yaml:"value,omitempty"`
	Lhs          CallArgument `yaml:"lhs,omitempty"`
	Func         int          `yaml:"func,omitempty"`

	// For assignments from function calls
	CalleeFunc  string `yaml:"callee_func,omitempty"`
	CalleePkg   string `yaml:"callee_pkg,omitempty"`
	ReturnIndex int    `yaml:"return_index,omitempty"`
}

// CallArgument represents a function call argument or expression
type CallArgument struct {
	idstr    string
	Kind     string                 `yaml:"kind"`            // ident, literal, selector, call, raw
	Name     string                 `yaml:"name,omitempty"`  // for ident
	Value    string                 `yaml:"value,omitempty"` // for literal
	X        *CallArgument          `yaml:"x,omitempty"`     // for selector/call
	Sel      *CallArgument          `yaml:"sel,omitempty"`   // for selector
	Fun      *CallArgument          `yaml:"fun,omitempty"`   // for call
	Args     []CallArgument         `yaml:"args,omitempty"`  // for call
	Raw      string                 `yaml:"raw,omitempty"`   // fallback
	Extra    map[string]interface{} `yaml:"extra,omitempty"` // extensibility
	Pkg      string                 `yaml:"pkg,omitempty"`   // for ident
	Type     string                 `yaml:"type,omitempty"`  // for ident
	Position string                 `yaml:"position,omitempty"`

	// Callee edge for the same call if it's kind is call
	Edge *CallGraphEdge `yaml:"-"`

	// New fields for argument-to-parameter and type parameter mapping
	ParamArgMap  map[string]CallArgument `yaml:"-"` // parameter name -> argument
	TypeParamMap map[string]string       `yaml:"-"` // type parameter name -> concrete type

	// NEW: Type parameter resolution information
	ResolvedType    string `yaml:"resolved_type,omitempty"`     // The concrete type after type parameter resolution
	IsGenericType   bool   `yaml:"is_generic_type,omitempty"`   // Whether this argument represents a generic type
	GenericTypeName string `yaml:"generic_type_name,omitempty"` // The generic type parameter name (e.g., "TRequest", "TData")
}

func (a *CallArgument) TypeParams() map[string]string {
	if a.TypeParamMap == nil {
		a.TypeParamMap = map[string]string{}
	}

	// Propagate type resolving
	if a.Edge != nil && len(a.Edge.TypeParamMap) > 0 {
		maps.Copy(a.TypeParamMap, a.Edge.TypeParamMap)
	}

	return a.TypeParamMap
}

func (a *CallArgument) ID() string {
	var pos string

	if a.idstr != "" {
		return a.idstr
	}

	if len(a.Position) > 0 {
		pos = "@" + a.Position
	}

	id, typeParam := a.id(".")

	a.idstr = id + typeParam + pos

	return a.idstr
}

// ID returns a unique identifier for the call argument
func (a *CallArgument) id(sep string) (string, string) {
	var typeParam string

	typeParams := a.TypeParams()
	if typeParams != nil {
		for _, param := range typeParams {
			exists := true

			var concreteType string

			for exists {
				concreteType, exists = typeParams[param]

				if concreteType != "" {
					param = concreteType
				}
			}
			typeParam += "," + param
		}

		if typeParam != "" {
			typeParam = fmt.Sprintf("[%s]", typeParam[1:])
		}
	}

	switch a.Kind {
	case kindIdent:
		// if a.Type != "" && a.Name == "" && sep == "/" {
		if a.Type != "" && sep == "/" {
			return a.Type, typeParam
		} else if a.Pkg != "" {
			if sep == "/" {
				return "", typeParam
			}
			return a.Pkg + sep + a.Name, typeParam
		}
		return a.Name, typeParam
	case kindLiteral:
		return a.Value, typeParam
	case kindSelector:
		if a.X != nil {
			xID, xTypeParam := a.X.id("/")
			if xID == "" {
				xID = a.Sel.Pkg
			}
			id := xID + sep + a.Sel.Name

			if xTypeParam != "" {
				typeParam = xTypeParam
			}

			return id, typeParam
		}
		return a.Sel.Name, typeParam
	case kindCall:
		if a.Fun != nil {
			funID, funTypeParam := a.Fun.id(".")
			if funTypeParam != "" {
				typeParam = funTypeParam
			}

			return funID, typeParam
		}
		return kindCall, typeParam
	case kindUnary:
		if a.X != nil {
			xID, xTypeParam := a.X.id("/")
			if xID == "" {
				xID = a.Pkg
			}
			id := a.Value + xID

			if xTypeParam != "" {
				typeParam = xTypeParam
			}

			return id, typeParam
		}
		return "", ""
	case kindCompositeLit:
		if a.X != nil {
			xID, xTypeParam := a.X.id("/")
			if xID == "" {
				xID = a.Pkg
			}
			id := xID

			if xTypeParam != "" {
				typeParam = xTypeParam
			}

			return id, typeParam
		}
		return "", ""
	case kindIndex:
		if a.X != nil {
			xID, xTypeParam := a.X.id("/")
			if xID == "" {
				xID = a.Pkg
			}
			id := xID

			if xTypeParam != "" {
				typeParam = xTypeParam
			}

			return id, typeParam
		}
		return "", ""
	default:
		return a.Raw, typeParam
	}
}

// Call represents a function call
type Call struct {
	Meta     *Metadata      `yaml:"-"`
	Edge     *CallGraphEdge `yaml:"-"`
	id       string         `yaml:"id"`
	Name     int            `yaml:"name,omitempty"`
	Pkg      int            `yaml:"pkg,omitempty"`
	Position int            `yaml:"position,omitempty"`
	RecvType int            `yaml:"recv_type,omitempty"`
}

// ID returns a unique identifier for the call
func (c Call) ID() string {
	var (
		pos       string
		typeParam string
	)

	if c.id != "" {
		return c.id
	}

	if c.Position >= 0 {
		pos = "@" + c.Meta.StringPool.GetString(c.Position)
	}

	if pos != "" {
		if c.Edge != nil && c.Edge.TypeParamMap != nil {
			for _, param := range c.Edge.TypeParamMap {
				exists := true

				var concreteType string

				for exists {
					concreteType, exists = c.Edge.TypeParamMap[param]

					if concreteType != "" {
						param = concreteType
					}
				}
				typeParam += "," + param
			}

			if typeParam != "" {
				typeParam = fmt.Sprintf("[%s]", typeParam[1:])
			}
		}
	}

	if c.RecvType != -1 {
		recvType := c.Meta.StringPool.GetString(c.RecvType)
		if strings.HasPrefix(recvType, "*") {
			recvType = "*" + c.Meta.StringPool.GetString(c.Pkg) + "." + recvType[1:]
		} else {
			recvType = c.Meta.StringPool.GetString(c.Pkg) + recvType
		}
		c.id = fmt.Sprintf("%s.%s%s%s", recvType, c.Meta.StringPool.GetString(c.Name), typeParam, pos)
	} else {
		c.id = fmt.Sprintf("%s.%s%s%s", c.Meta.StringPool.GetString(c.Pkg), c.Meta.StringPool.GetString(c.Name), typeParam, pos)
	}

	return c.id
}

// CallGraphEdge represents an edge in the call graph
type CallGraphEdge struct {
	Caller        Call                    `yaml:"caller,omitempty"`
	Callee        Call                    `yaml:"callee,omitempty"`
	Position      int                     `yaml:"position,omitempty"`
	Args          []CallArgument          `yaml:"args,omitempty"`
	AssignmentMap map[string][]Assignment `yaml:"assignments,omitempty"`

	// New fields for argument-to-parameter and type parameter mapping
	ParamArgMap  map[string]CallArgument `yaml:"param_arg_map,omitempty"`  // parameter name -> argument
	TypeParamMap map[string]string       `yaml:"type_param_map,omitempty"` // type parameter name -> concrete type

	CalleeVarName     string `yaml:"callee_var_name,omitempty"`
	CalleeRecvVarName string `yaml:"callee_recv_var_name,omitempty"`

	meta *Metadata
}

func (edge *CallGraphEdge) NewCall(name, pkg, position, recvType int) *Call {
	return &Call{
		Edge:     edge,
		Meta:     edge.meta,
		Name:     name,
		Pkg:      pkg,
		Position: position,
		RecvType: recvType,
	}
}

// GlobalAssignment represents a global variable assignment
type GlobalAssignment struct {
	ConcreteType string `yaml:"-"`
	PkgName      string `yaml:"-"`
}
