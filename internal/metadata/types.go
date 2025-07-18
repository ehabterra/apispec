package metadata

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
	StringPool *StringPool         `yaml:"string_pool"`
	Packages   map[string]*Package `yaml:"packages"`
	CallGraph  []CallGraphEdge     `yaml:"call_graph"`
}

// Package represents a Go package
type Package struct {
	ImportPath int              `yaml:"import_path"`
	Files      map[string]*File `yaml:"files"`
	Types      map[string]*Type `yaml:"types"`
}

// File represents a Go source file
type File struct {
	Types           map[string]*Type     `yaml:"types"`
	Functions       map[string]*Function `yaml:"functions"`
	Variables       map[string]*Variable `yaml:"variables"`
	StructInstances []StructInstance     `yaml:"struct_instances"`
	Selectors       []Selector           `yaml:"selectors"`
	Imports         map[int]int          `yaml:"imports"` // alias -> path
}

// Type represents a Go type
type Type struct {
	Name          int      `yaml:"name"`
	Kind          int      `yaml:"kind"`
	Target        int      `yaml:"target"`
	Implements    []int    `yaml:"implements"`
	ImplementedBy []int    `yaml:"implemented_by"`
	Embeds        []int    `yaml:"embeds"`
	Fields        []Field  `yaml:"fields"`
	Scope         int      `yaml:"scope"`
	Methods       []Method `yaml:"methods"`
	Comments      int      `yaml:"comments"`
	Tags          []int    `yaml:"tags"`
}

// Field represents a struct field
type Field struct {
	Name     int `yaml:"name"`
	Type     int `yaml:"type"`
	Tag      int `yaml:"tag"`
	Scope    int `yaml:"scope"`
	Comments int `yaml:"comments"`
}

// Method represents a method
type Method struct {
	Name         int          `yaml:"name"`
	Receiver     int          `yaml:"receiver"`
	Signature    CallArgument `yaml:"signature"`
	SignatureStr int          `yaml:"signature_str"`
	Position     int          `yaml:"position"`
	Scope        int          `yaml:"scope"`
	Comments     int          `yaml:"comments"`
	Tags         []int        `yaml:"tags"`

	// map of variable name to all assignments (for alias/reassignment tracking)
	AssignmentMap map[string][]Assignment `yaml:"assignment_map,omitempty"`
}

// Function represents a function
type Function struct {
	Name      int          `yaml:"name"`
	Signature CallArgument `yaml:"signature"`
	Position  int          `yaml:"position"`
	Scope     int          `yaml:"scope"`
	Comments  int          `yaml:"comments"`
	Tags      []int        `yaml:"tags"`

	// Type parameter names for generics
	TypeParams []string `yaml:"type_params,omitempty"`

	// Return value origins for tracing through return values
	ReturnVars []CallArgument `yaml:"return_vars,omitempty"`

	// map of variable name to all assignments (for alias/reassignment tracking)
	AssignmentMap map[string][]Assignment `yaml:"assignment_map,omitempty"`
}

// Variable represents a variable
type Variable struct {
	Name     int `yaml:"name"`
	Tok      int `yaml:"tok"`
	Type     int `yaml:"type"`
	Value    int `yaml:"value"`
	Position int `yaml:"position"`
	Comments int `yaml:"comments"`
}

// Selector represents a selector expression
type Selector struct {
	Expr     CallArgument `yaml:"expr"`
	Kind     int          `yaml:"kind"`
	Position int          `yaml:"position"`
}

// StructInstance represents a struct literal instance
type StructInstance struct {
	Type     int         `yaml:"type"`
	Position int         `yaml:"position"`
	Fields   map[int]int `yaml:"fields"`
}

// Assignment represents a variable assignment
type Assignment struct {
	VariableName int          `yaml:"variable_name"`
	Pkg          int          `yaml:"pkg"`
	ConcreteType int          `yaml:"concrete_type"`
	Position     int          `yaml:"position"`
	Scope        int          `yaml:"scope"`
	Value        CallArgument `yaml:"value"`

	// For assignments from function calls
	CalleeFunc  string `yaml:"callee_func,omitempty"`
	CalleePkg   string `yaml:"callee_pkg,omitempty"`
	ReturnIndex int    `yaml:"return_index,omitempty"`
}

// CallArgument represents a function call argument or expression
type CallArgument struct {
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
}

func (a *CallArgument) ID() string {
	return a.id(".")
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
	meta *Metadata

	Name     int `yaml:"name"`
	Pkg      int `yaml:"pkg"`
	RecvType int `yaml:"recv_type"`
}

// ID returns a unique identifier for the call
func (c Call) ID() string {
	return c.meta.StringPool.GetString(c.Pkg) + "." + c.meta.StringPool.GetString(c.Name)
}

// CallGraphEdge represents an edge in the call graph
type CallGraphEdge struct {
	Caller        Call                    `yaml:"caller"`
	Callee        Call                    `yaml:"callee"`
	Position      int                     `yaml:"position"`
	Args          []CallArgument          `yaml:"args"`
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
	ConcreteType string
	PkgName      string
}
