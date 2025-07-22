package metadata

import "fmt"

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
}

// Package represents a Go package
type Package struct {
	ImportPath int              `yaml:"import_path,omitempty"`
	Files      map[string]*File `yaml:"files,omitempty"`
	Types      map[string]*Type `yaml:"types,omitempty"`
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
	AssignmentMap map[string][]Assignment `yaml:"-"`
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
	AssignmentMap map[string][]Assignment `yaml:"-"`
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
	Sel      string                 `yaml:"sel,omitempty"`   // for selector
	Fun      *CallArgument          `yaml:"fun,omitempty"`   // for call
	Args     []CallArgument         `yaml:"args,omitempty"`  // for call
	Raw      string                 `yaml:"raw,omitempty"`   // fallback
	Extra    map[string]interface{} `yaml:"extra,omitempty"` // extensibility
	Pkg      string                 `yaml:"pkg,omitempty"`   // for ident
	Type     string                 `yaml:"type,omitempty"`  // for ident
	Position string                 `yaml:"position,omitempty"`

	// New fields for argument-to-parameter and type parameter mapping
	ParamArgMap  map[string]CallArgument `yaml:"-"` // parameter name -> argument
	TypeParamMap map[string]string       `yaml:"-"` // type parameter name -> concrete type

	// NEW: Type parameter resolution information
	ResolvedType    string `yaml:"resolved_type,omitempty"`     // The concrete type after type parameter resolution
	IsGenericType   bool   `yaml:"is_generic_type,omitempty"`   // Whether this argument represents a generic type
	GenericTypeName string `yaml:"generic_type_name,omitempty"` // The generic type parameter name (e.g., "TRequest", "TData")
}

func (a *CallArgument) ID() string {
	var pos string

	if a.idstr != "" {
		return a.idstr
	}

	if len(a.Position) > 0 {
		pos = "@" + a.Position
	}

	a.idstr = a.id(".") + pos

	return a.idstr
}

// ID returns a unique identifier for the call argument
func (a *CallArgument) id(sep string) string {
	switch a.Kind {
	case kindIdent:
		if a.Pkg != "" {
			return a.Pkg + sep + a.Name
		}
		return a.Name
	case kindLiteral:
		return a.Value
	case kindSelector:
		if a.X != nil {
			return a.X.id("/") + sep + a.Sel
		}
		return a.Sel
	case kindCall:
		if a.Fun != nil {
			return a.Fun.ID()
		}
		return kindCall
	default:
		return a.Raw
	}
}

// Call represents a function call
type Call struct {
	Meta     *Metadata `yaml:"-"`
	id       string
	Name     int `yaml:"name,omitempty"`
	Pkg      int `yaml:"pkg,omitempty"`
	Position int `yaml:"position,omitempty"`
	RecvType int `yaml:"recv_type,omitempty"`
}

// ID returns a unique identifier for the call
func (c Call) ID() string {
	var pos string

	if c.id != "" {
		return c.id
	}

	if c.Position >= 0 {
		pos = "@" + c.Meta.StringPool.GetString(c.Position)
	}

	c.id = fmt.Sprintf("%s.%s%s", c.Meta.StringPool.GetString(c.Pkg), c.Meta.StringPool.GetString(c.Name), pos)

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

// GlobalAssignment represents a global variable assignment
type GlobalAssignment struct {
	ConcreteType string `yaml:"-"`
	PkgName      string `yaml:"-"`
}
