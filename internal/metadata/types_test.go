package metadata

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"
)

func TestProcessFunctionReturnTypes(t *testing.T) {
	// Test source code with functions and methods that return different types
	src := `
package main

type User struct {
	Name string
	Age  int
}

func NewUser(name string, age int) *User {
	return &User{Name: name, Age: age}
}

func GetUserName(user *User) string {
	return user.Name
}

func (u *User) GetAge() int {
	return u.Age
}

func (u *User) GetInfo() (string, int) {
	return u.Name, u.Age
}

var globalUser = &User{Name: "John", Age: 30}

func GetGlobalUser() *User {
	return globalUser
}

func CreateString() string {
	return "hello"
}

func CreateInt() int {
	return 42
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pkgs := map[string]map[string]*ast.File{"main": {"test.go": file}}
	importPaths := map[string]string{"main": "main"}
	fileToInfo := map[*ast.File]*types.Info{}

	// Generate metadata
	metadata := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)

	// Test that functions have resolved types
	tests := []struct {
		name         string
		expectedType string
	}{
		{"NewUser", "*User"},
		{"GetUserName", "string"},
		{"GetGlobalUser", "*User"},
		{"CreateString", "string"},
		{"CreateInt", "int"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, pkg := range metadata.Packages {
				for _, file := range pkg.Files {
					if fn, exists := file.Functions[tt.name]; exists {
						resolvedType := fn.Signature.GetResolvedType()
						if resolvedType != tt.expectedType {
							t.Errorf("function %s: expected resolved type %q, got %q",
								tt.name, tt.expectedType, resolvedType)
						}
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Errorf("function %s not found", tt.name)
			}
		})
	}

	// Test that methods have resolved types
	methodTests := []struct {
		receiverType string
		methodName   string
		expectedType string
	}{
		{"User", "GetAge", "int"},
		{"User", "GetInfo", "string"}, // First return value
	}

	for _, tt := range methodTests {
		t.Run(tt.receiverType+"."+tt.methodName, func(t *testing.T) {
			found := false
			for _, pkg := range metadata.Packages {
				for _, file := range pkg.Files {
					if typ, exists := file.Types[tt.receiverType]; exists {
						for _, method := range typ.Methods {
							if metadata.StringPool.GetString(method.Name) == tt.methodName {
								resolvedType := method.Signature.GetResolvedType()
								if resolvedType != tt.expectedType {
									t.Errorf("method %s.%s: expected resolved type %q, got %q",
										tt.receiverType, tt.methodName, tt.expectedType, resolvedType)
								}
								found = true
								break
							}
						}
					}
					if found {
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Errorf("method %s.%s not found", tt.receiverType, tt.methodName)
			}
		})
	}
}

func TestProcessFunctionReturnTypes_ComplexTypes(t *testing.T) {
	// Test with more complex return types
	src := `
package main

type Config struct {
	Host string
	Port int
}

type Server struct {
	config *Config
}

func NewConfig(host string, port int) *Config {
	return &Config{Host: host, Port: port}
}

func NewServer(config *Config) *Server {
	return &Server{config: config}
}

func (s *Server) GetConfig() *Config {
	return s.config
}

func (s *Server) GetHost() string {
	return s.config.Host
}

func CreateMap() map[string]int {
	return map[string]int{"a": 1, "b": 2}
}

func CreateSlice() []string {
	return []string{"a", "b", "c"}
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pkgs := map[string]map[string]*ast.File{"main": {"test.go": file}}
	importPaths := map[string]string{"main": "main"}
	fileToInfo := map[*ast.File]*types.Info{}

	metadata := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)

	// Test complex types
	tests := []struct {
		name         string
		expectedType string
	}{
		{"NewConfig", "*Config"},
		{"NewServer", "*Server"},
		{"CreateMap", "map[string]int"},
		{"CreateSlice", "[]string"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, pkg := range metadata.Packages {
				for _, file := range pkg.Files {
					if fn, exists := file.Functions[tt.name]; exists {
						resolvedType := fn.Signature.GetResolvedType()
						if resolvedType != tt.expectedType {
							t.Errorf("function %s: expected resolved type %q, got %q",
								tt.name, tt.expectedType, resolvedType)
						}
						found = true
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Errorf("function %s not found", tt.name)
			}
		})
	}

	// Test method with selector return
	methodTests := []struct {
		receiverType string
		methodName   string
		expectedType string
	}{
		{"Server", "GetConfig", "*Config"},
		{"Server", "GetHost", "string"},
	}

	for _, tt := range methodTests {
		t.Run(tt.receiverType+"."+tt.methodName, func(t *testing.T) {
			found := false
			for _, pkg := range metadata.Packages {
				for _, file := range pkg.Files {
					if typ, exists := file.Types[tt.receiverType]; exists {
						for _, method := range typ.Methods {
							if metadata.StringPool.GetString(method.Name) == tt.methodName {
								resolvedType := method.Signature.GetResolvedType()
								if resolvedType != tt.expectedType {
									t.Errorf("method %s.%s: expected resolved type %q, got %q",
										tt.receiverType, tt.methodName, tt.expectedType, resolvedType)
								}
								found = true
								break
							}
						}
					}
					if found {
						break
					}
				}
				if found {
					break
				}
			}
			if !found {
				t.Errorf("method %s.%s not found", tt.receiverType, tt.methodName)
			}
		})
	}
}

func TestProcessFunctionReturnTypes_Debug(t *testing.T) {
	// Test source code with a map function
	src := `
package main

func CreateMap() map[string]int {
	return map[string]int{"a": 1, "b": 2}
}
`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}

	pkgs := map[string]map[string]*ast.File{"main": {"test.go": file}}
	importPaths := map[string]string{"main": "main"}
	fileToInfo := map[*ast.File]*types.Info{}

	// Generate metadata
	metadata := GenerateMetadata(pkgs, fileToInfo, importPaths, fset)

	// Debug: Print the signature details
	for _, pkg := range metadata.Packages {
		for _, file := range pkg.Files {
			if fn, exists := file.Functions["CreateMap"]; exists {
				t.Logf("Function signature kind: %s", fn.Signature.GetKind())
				t.Logf("Function signature type: %s", fn.Signature.GetType())
				t.Logf("Function signature raw: %s", fn.Signature.GetRaw())
				if fn.Signature.Fun != nil {
					t.Logf("Function signature Fun kind: %s", fn.Signature.Fun.GetKind())
					t.Logf("Function signature Fun type: %s", fn.Signature.Fun.GetType())
					t.Logf("Function signature Fun Args length: %d", len(fn.Signature.Fun.Args))
					if len(fn.Signature.Fun.Args) > 0 {
						t.Logf("Function signature Fun Args[0] kind: %s", fn.Signature.Fun.Args[0].GetKind())
						t.Logf("Function signature Fun Args[0] type: %s", fn.Signature.Fun.Args[0].GetType())
					}
				}
				if len(fn.Signature.Args) > 0 {
					t.Logf("Function signature Args[0] kind: %s", fn.Signature.Args[0].GetKind())
					t.Logf("Function signature Args[0] type: %s", fn.Signature.Args[0].GetType())
				}
				resolvedType := fn.Signature.GetResolvedType()
				t.Logf("Resolved type: %s", resolvedType)
				break
			}
		}
	}
}

func TestProcessFunctionReturnTypes_CallGraph(t *testing.T) {
	// Test that the processCallGraphReturnTypes method works correctly
	metadata := &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
		CallGraph:  make([]CallGraphEdge, 0),
	}

	// Create a test package with functions
	pkg := &Package{
		Files: make(map[string]*File),
	}

	file := &File{
		Functions: make(map[string]*Function),
	}

	// Create test functions
	newUserFunc := &Function{
		Name: metadata.StringPool.Get("NewUser"),
		Signature: CallArgument{
			Kind: metadata.StringPool.Get(KindFuncType),
			Meta: metadata,
		},
	}
	newUserFunc.Signature.SetResolvedType("*User")
	file.Functions["NewUser"] = newUserFunc

	getUserNameFunc := &Function{
		Name: metadata.StringPool.Get("GetUserName"),
		Signature: CallArgument{
			Kind: metadata.StringPool.Get(KindFuncType),
			Meta: metadata,
		},
	}
	getUserNameFunc.Signature.SetResolvedType("string")
	file.Functions["GetUserName"] = getUserNameFunc

	pkg.Files["test.go"] = file
	metadata.Packages["main"] = pkg

	// Create test call graph edges
	caller := Call{
		Meta: metadata,
		Name: metadata.StringPool.Get("main"),
		Pkg:  metadata.StringPool.Get("main"),
	}

	callee := Call{
		Meta: metadata,
		Name: metadata.StringPool.Get("NewUser"),
		Pkg:  metadata.StringPool.Get("main"),
	}

	// Create a function call argument
	newUserCall := CallArgument{
		Kind: metadata.StringPool.Get(KindCall),
		Fun: &CallArgument{
			Kind: metadata.StringPool.Get(KindIdent),
			Name: metadata.StringPool.Get("NewUser"),
			Meta: metadata,
		},
		Meta: metadata,
	}

	edge := CallGraphEdge{
		Caller: caller,
		Callee: callee,
		Args:   []CallArgument{newUserCall},
		meta:   metadata,
	}

	metadata.CallGraph = append(metadata.CallGraph, edge)

	// Process the call graph
	metadata.processCallGraphReturnTypes()

	// Check that the function call argument has ResolvedType set
	for _, edge := range metadata.CallGraph {
		for _, arg := range edge.Args {
			if arg.GetKind() == KindCall && arg.Fun != nil {
				funcName := arg.Fun.GetName()
				resolvedType := arg.GetResolvedType()

				if funcName == "NewUser" && resolvedType != "*User" {
					t.Errorf("NewUser call: expected resolved type %q, got %q", "*User", resolvedType)
				}
			}
		}
	}
}

func TestResolvedTypeInResolveTypeOrigin(t *testing.T) {
	// Test that demonstrates how resolveTypeOrigin can now benefit from ResolvedType
	metadata := &Metadata{
		StringPool: NewStringPool(),
		Packages:   make(map[string]*Package),
		CallGraph:  make([]CallGraphEdge, 0),
	}

	// Create a test package with functions
	pkg := &Package{
		Files: make(map[string]*File),
	}

	file := &File{
		Functions: make(map[string]*Function),
	}

	// Create a test function with resolved type
	createUserFunc := &Function{
		Name: metadata.StringPool.Get("CreateUser"),
		Signature: CallArgument{
			Kind: metadata.StringPool.Get(KindFuncType),
			Meta: metadata,
		},
	}
	createUserFunc.Signature.SetResolvedType("*User")
	file.Functions["CreateUser"] = createUserFunc

	pkg.Files["test.go"] = file
	metadata.Packages["main"] = pkg

	// Create a function call argument with ResolvedType set
	funcCallArg := CallArgument{
		Kind: metadata.StringPool.Get(KindCall),
		Fun: &CallArgument{
			Kind: metadata.StringPool.Get(KindIdent),
			Name: metadata.StringPool.Get("CreateUser"),
			Meta: metadata,
		},
		Meta: metadata,
	}
	funcCallArg.SetResolvedType("*User") // This is what we're testing

	// Simulate the resolveTypeOrigin logic
	// This mimics what happens in the pattern matchers
	resolvedType := ""
	if funcCallArg.ResolvedType != -1 {
		resolvedType = funcCallArg.GetResolvedType()
	}

	// Verify that we get the resolved type
	if resolvedType != "*User" {
		t.Errorf("Expected resolved type %q, got %q", "*User", resolvedType)
	}

	// Test with a different type
	funcCallArg2 := CallArgument{
		Kind: metadata.StringPool.Get(KindCall),
		Fun: &CallArgument{
			Kind: metadata.StringPool.Get(KindIdent),
			Name: metadata.StringPool.Get("GetName"),
			Meta: metadata,
		},
		Meta: metadata,
	}
	funcCallArg2.SetResolvedType("string")

	resolvedType2 := ""
	if funcCallArg2.ResolvedType != -1 {
		resolvedType2 = funcCallArg2.GetResolvedType()
	}

	if resolvedType2 != "string" {
		t.Errorf("Expected resolved type %q, got %q", "string", resolvedType2)
	}

	// Test with no resolved type (should return empty string)
	funcCallArg3 := CallArgument{
		Kind: metadata.StringPool.Get(KindCall),
		Fun: &CallArgument{
			Kind: metadata.StringPool.Get(KindIdent),
			Name: metadata.StringPool.Get("UnknownFunc"),
			Meta: metadata,
		},
		Meta:         metadata,
		ResolvedType: -1, // Explicitly set to -1 to test the fallback case
	}

	resolvedType3 := ""
	if funcCallArg3.ResolvedType != -1 {
		resolvedType3 = funcCallArg3.GetResolvedType()
	}

	if resolvedType3 != "" {
		t.Errorf("Expected empty resolved type, got %q", resolvedType3)
	}
}
