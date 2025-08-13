package metadata_test

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"strings"
	"testing"

	"github.com/ehabterra/swagen/internal/metadata"
	"github.com/stretchr/testify/assert"
	"golang.org/x/tools/go/packages"
	"golang.org/x/tools/go/packages/packagestest"
)

func TestGenerateMetadata(t *testing.T) {
	fset := token.NewFileSet()

	type Variable struct {
		TokType string
		Name    string
		Type    string
		Value   string
	}

	type Field struct {
		Name string
		Type string
		Tag  string
	}

	type Method struct {
		Name     string
		Receiver string
	}

	type Type struct {
		Name          string
		Kind          string
		ImplementedBy []string
		Implements    []string
		Fields        []Field
		Methods       []Method
		Embeds        []string
	}

	type Import struct {
		Alias string
		Path  string
	}

	type StructInstance struct {
		Type   string
		Fields map[string]string
	}

	type Function struct {
		Name        string
		Assignments []string
		TypeParams  []string
		ReturnVars  int // Number of return variables tracked
	}

	type File struct {
		Name            string
		Functions       []Function
		Imports         []Import
		Types           []Type
		Variables       []Variable
		StructInstances []StructInstance
	}

	type Package struct {
		Name  string
		Files []File
	}

	type Edge struct {
		Caller        string
		Callee        string
		Args          []string
		ParamCount    int
		TypeParams    map[string]string
		AssignmentMap map[string][]string
	}

	type Expected struct {
		Packages  []Package
		CallGraph []Edge
	}

	type testCase struct {
		name     string
		src      []packagestest.Module
		expected Expected
	}

	tests := []testCase{
		{
			name: "Simple function with variables and imports",
			src: []packagestest.Module{{
				Name: "main",
				Files: map[string]interface{}{
					"test.go": `package main

import (
	"fmt"
	"strings"
)

func main() {
	x := 1
	y := "hello"
	z := fmt.Sprintf("%d", x)
	fmt.Println(z)
	strings.ToUpper(y)
}`,
				}}},
			expected: Expected{
				Packages: []Package{
					{
						Name: "main",
						Files: []File{
							{
								Name: "test.go",
								Functions: []Function{
									{Name: "main", Assignments: []string{"x", "y", "z"}},
								},
								Imports: []Import{
									{Alias: "fmt", Path: "fmt"},
									{Alias: "strings", Path: "strings"},
								},
							},
						},
					},
				},
				CallGraph: []Edge{
					{Caller: "main", Callee: "Sprintf", Args: []string{`"%d"`, "x"}},
					{Caller: "main", Callee: "Println", Args: []string{"z"}},
					{Caller: "main", Callee: "ToUpper", Args: []string{"y"}},
				},
			},
		},
		{
			name: "Struct types with methods and interfaces",
			src: []packagestest.Module{{
				Name: "example",
				Files: map[string]interface{}{
					"types.go": `package example

type User struct {
	Name string ` + "`json:\"name\"`" + `
	Age  int    ` + "`json:\"age\"`" + `
}

type Namer interface {
	GetName() string
}

type Ager interface {
	SetAge(age int)
}

type Phoner interface {
	SetPhone(numb string)
}

func (u *User) GetName() string {
	return u.Name
}

func (u *User) SetAge(age int) {
	u.Age = age
}

func NewUser(name string, age int) *User {
	u := &User{
		Name: name,
		Age:  age,
	}

	u.SetAge(age)
	
	return u
}
	
func main() {
	user := NewUser("User Name", 40)
	fmt.Println(*user)
}`,
				}}},
			expected: Expected{
				Packages: []Package{
					{
						Name: "example",
						Files: []File{
							{
								Name: "types.go",
								Types: []Type{
									{
										Name:       "User",
										Kind:       "struct",
										Implements: []string{"example.Namer", "example.Ager"},
										Fields: []Field{
											{Name: "Name", Type: "string", Tag: `json:"name"`},
											{Name: "Age", Type: "int", Tag: `json:"age"`},
										},
										Methods: []Method{
											{Name: "GetName", Receiver: "*User"},
											{Name: "SetAge", Receiver: "*User"},
										},
									},
									{
										Name:          "Namer",
										Kind:          "interface",
										ImplementedBy: []string{"example.User"},
										Methods: []Method{
											{Name: "GetName"},
										},
									},
									{
										Name:          "Ager",
										Kind:          "interface",
										ImplementedBy: []string{"example.User"},
										Methods: []Method{
											{Name: "SetAge"},
										},
									},
									{
										Name: "Phoner",
										Kind: "interface",
										Methods: []Method{
											{Name: "SetPhone"},
										},
									},
								},
								Functions: []Function{
									{Name: "NewUser", ReturnVars: 1},
									{Name: "main", Assignments: []string{"user"}},
								},
								StructInstances: []StructInstance{
									{
										Type: "User",
										Fields: map[string]string{
											"Name": "name",
											"Age":  "age",
										},
									},
								},
							},
						},
					},
				},
				CallGraph: []Edge{
					{Caller: "main", Callee: "NewUser", Args: []string{`"User Name"`, "40"}},
					{Caller: "main", Callee: "Println", Args: []string{"user"}},
				},
			},
		},
		{
			name: "Generic functions and types",
			src: []packagestest.Module{{
				Name: "generic",
				Files: map[string]interface{}{
					"generic.go": `package generic

type Container[T any] struct {
	Value T
}

func (c *Container[T]) Get() T {
	return c.Value
}

func (c *Container[T]) Set(value T) {
	c.Value = value
}

func NewContainer[T any](value T) *Container[T] {
	return &Container[T]{Value: value}
}

func Process[T comparable](items []T) T {
	var zero T
	if len(items) == 0 {
		return zero
	}
	return items[0]
}

func main() {
	c := NewContainer[int](42)
	val := c.Get()
	c.Set(100)
	
	result := Process[string]([]string{"hello", "world"})
	_ = val
	_ = result
}`,
				}}},
			expected: Expected{
				Packages: []Package{
					{
						Name: "generic",
						Files: []File{
							{
								Name: "generic.go",
								Types: []Type{
									{
										Name: "Container",
										Kind: "struct",
										Fields: []Field{
											{Name: "Value", Type: "T"},
										},
										Methods: []Method{
											{Name: "Get", Receiver: "*Container[T]"},
											{Name: "Set", Receiver: "*Container[T]"},
										},
									},
								},
								Functions: []Function{
									{Name: "NewContainer", TypeParams: []string{"T"}, ReturnVars: 1},
									{Name: "Process", TypeParams: []string{"T"}, ReturnVars: 1},
									{Name: "main", Assignments: []string{"c", "val", "result"}},
								},
							},
						},
					},
				},
				CallGraph: []Edge{
					{Caller: "main", Callee: "NewContainer", TypeParams: map[string]string{"T": "int"}},
					{Caller: "main", Callee: "Get"},
					{Caller: "main", Callee: "Set", Args: []string{"100"}},
					{Caller: "main", Callee: "Process", TypeParams: map[string]string{"T": "string"}},
				},
			},
		},
		{
			name: "Constants and variables",
			src: []packagestest.Module{{
				Name: "constants",
				Files: map[string]interface{}{
					"const.go": `package constants

const (
	MaxSize = 100
	Name    = "test"
	Pi      = 3.14
)

var (
	GlobalVar int = 42
	Message   string
)

func init() {
	Message = "initialized"
}

func UseConstants() {
	size := MaxSize
	name := Name
	_ = size
	_ = name
}`,
				}}},
			expected: Expected{
				Packages: []Package{
					{
						Name: "constants",
						Files: []File{
							{
								Name: "const.go",
								Variables: []Variable{
									{TokType: "const", Name: "MaxSize", Type: "", Value: "100"},
									{TokType: "const", Name: "Name", Type: "", Value: `test`},
									{TokType: "const", Name: "Pi", Type: "", Value: "3.14"},
									{TokType: "var", Name: "GlobalVar", Type: "int", Value: "42"},
									{TokType: "var", Name: "Message", Type: "string", Value: ""},
								},
								Functions: []Function{
									{Name: "init", Assignments: []string{"Message"}},
									{Name: "UseConstants", Assignments: []string{"size", "name"}},
								},
							},
						},
					},
				},
			},
		},
		{
			name: "Complex call graph with method chains",
			src: []packagestest.Module{{
				Name: "complex",
				Files: map[string]interface{}{
					"service.go": `package complex

import "fmt"

type Service struct {
	name string
}

func (s *Service) GetName() string {
	return s.name
}

func (s *Service) Process(data string) string {
	result := fmt.Sprintf("Processing: %s", data)
	return result
}

type Handler struct {
	service *Service
}

func (h *Handler) Handle(input string) {
	name := h.service.GetName()
	processed := h.service.Process(input)
	fmt.Printf("Handler %s: %s\n", name, processed)
}

func NewService(name string) *Service {
	return &Service{name: name}
}

func NewHandler(svc *Service) *Handler {
	return &Handler{service: svc}
}

func main() {
	svc := NewService("test-service")
	handler := NewHandler(svc)
	handler.Handle("test-data")
}`,
				}}},
			expected: Expected{
				Packages: []Package{
					{
						Name: "complex",
						Files: []File{
							{
								Name: "service.go",
								Types: []Type{
									{
										Name: "Service",
										Kind: "struct",
										Fields: []Field{
											{Name: "name", Type: "string"},
										},
										Methods: []Method{
											{Name: "GetName", Receiver: "*Service"},
											{Name: "Process", Receiver: "*Service"},
										},
									},
									{
										Name: "Handler",
										Kind: "struct",
										Fields: []Field{
											{Name: "service", Type: "*Service"},
										},
										Methods: []Method{
											{Name: "Handle", Receiver: "*Handler"},
										},
									},
								},
								Functions: []Function{
									{Name: "NewService", ReturnVars: 1},
									{Name: "NewHandler", ReturnVars: 1},
									{Name: "main", Assignments: []string{"svc", "handler"}},
								},
								Imports: []Import{
									{Alias: "fmt", Path: "fmt"},
								},
								StructInstances: []StructInstance{
									{Type: "Service", Fields: map[string]string{"name": "name"}},
									{Type: "Handler", Fields: map[string]string{"service": "svc"}},
								},
							},
						},
					},
				},
				CallGraph: []Edge{
					{Caller: "main", Callee: "NewService", Args: []string{`"test-service"`}, AssignmentMap: map[string][]string{"svc": {"NewService"}}},
					{Caller: "main", Callee: "NewHandler", Args: []string{"svc"}, AssignmentMap: map[string][]string{"handler": {"NewHandler"}}},
					{Caller: "main", Callee: "Handle", Args: []string{`"test-data"`}},
					{Caller: "Handle", Callee: "GetName"},
					{Caller: "Handle", Callee: "Process"},
					{Caller: "Handle", Callee: "Printf"},
					{Caller: "Process", Callee: "Sprintf"},
				},
			},
		},
		{
			name: "Multi-package with cross-package dependencies",
			src: []packagestest.Module{{
				Name: "multipackage",
				Files: map[string]interface{}{
					"main.go": `package main

import (
	"fmt"
	"multipackage/models"
	"multipackage/services"
)

func main() {
	user := models.NewUser("John", 25)
	service := services.NewUserService()
	
	result := service.ProcessUser(user)
	fmt.Println(result)
}`,
					"models/user.go": `package models

type User struct {
	Name string ` + "`json:\"name\"`" + `
	Age  int    ` + "`json:\"age\"`" + `
}

type UserInterface interface {
	GetName() string
	GetAge() int
}

func (u *User) GetName() string {
	return u.Name
}

func (u *User) GetAge() int {
	return u.Age
}

func NewUser(name string, age int) *User {
	return &User{
		Name: name,
		Age:  age,
	}
}`,
					"services/user_service.go": `package services

import (
	"fmt"
	"multipackage/models"
)

type UserService struct {
	prefix string
}

func NewUserService() *UserService {
	return &UserService{prefix: "User:"}
}

func (s *UserService) ProcessUser(user *models.User) string {
	name := user.GetName()
	age := user.GetAge()
	return fmt.Sprintf("%s %s is %d years old", s.prefix, name, age)
}

func (s *UserService) SetPrefix(prefix string) {
	s.prefix = prefix
}`,
					"utils/helper.go": `package utils

import "strings"

func FormatString(input string) string {
	return strings.ToUpper(strings.TrimSpace(input))
}

func ValidateAge(age int) bool {
	return age > 0 && age < 150
}

const (
	MinAge = 1
	MaxAge = 149
)

var DefaultFormat = "uppercase"`,
				}}},
			expected: Expected{
				Packages: []Package{
					{
						Name: "multipackage",
						Files: []File{
							{
								Name: "main.go",
								Functions: []Function{
									{Name: "main", Assignments: []string{"user", "service", "result"}},
								},
								Imports: []Import{
									{Alias: "fmt", Path: "fmt"},
									{Alias: "multipackage/models", Path: "multipackage/models"},
									{Alias: "multipackage/services", Path: "multipackage/services"},
								},
							},
						},
					},
					{
						Name: "multipackage/models",
						Files: []File{
							{
								Name: "user.go",
								Types: []Type{
									{
										Name:       "User",
										Kind:       "struct",
										Implements: []string{"multipackage/models.UserInterface"},
										Fields: []Field{
											{Name: "Name", Type: "string", Tag: `json:"name"`},
											{Name: "Age", Type: "int", Tag: `json:"age"`},
										},
										Methods: []Method{
											{Name: "GetName", Receiver: "*User"},
											{Name: "GetAge", Receiver: "*User"},
										},
									},
									{
										Name:          "UserInterface",
										Kind:          "interface",
										ImplementedBy: []string{"multipackage/models.User"},
										Methods: []Method{
											{Name: "GetName"},
											{Name: "GetAge"},
										},
									},
								},
								Functions: []Function{
									{Name: "NewUser", ReturnVars: 1},
								},
								StructInstances: []StructInstance{
									{
										Type: "User",
										Fields: map[string]string{
											"Name": "name",
											"Age":  "age",
										},
									},
								},
							},
						},
					},
					{
						Name: "multipackage/services",
						Files: []File{
							{
								Name: "user_service.go",
								Types: []Type{
									{
										Name: "UserService",
										Kind: "struct",
										Fields: []Field{
											{Name: "prefix", Type: "string"},
										},
										Methods: []Method{
											{Name: "ProcessUser", Receiver: "*UserService"},
											{Name: "SetPrefix", Receiver: "*UserService"},
										},
									},
								},
								Functions: []Function{
									{Name: "NewUserService", ReturnVars: 1},
								},
								Imports: []Import{
									{Alias: "fmt", Path: "fmt"},
									{Alias: "multipackage/models", Path: "multipackage/models"},
								},
								StructInstances: []StructInstance{
									{
										Type: "UserService",
										Fields: map[string]string{
											"prefix": "User:",
										},
									},
								},
							},
						},
					},
					{
						Name: "multipackage/utils",
						Files: []File{
							{
								Name: "helper.go",
								Functions: []Function{
									{Name: "FormatString", ReturnVars: 1},
									{Name: "ValidateAge", ReturnVars: 1},
								},
								Imports: []Import{
									{Alias: "strings", Path: "strings"},
								},
								Variables: []Variable{
									{TokType: "const", Name: "MinAge", Type: "", Value: "1"},
									{TokType: "const", Name: "MaxAge", Type: "", Value: "149"},
									{TokType: "var", Name: "DefaultFormat", Type: "", Value: "uppercase"},
								},
							},
						},
					},
				},
				CallGraph: []Edge{
					{Caller: "main", Callee: "NewUser", Args: []string{`"John"`, "25"}},
					{Caller: "main", Callee: "NewUserService"},
					{Caller: "main", Callee: "ProcessUser", Args: []string{"user"}},
					{Caller: "main", Callee: "Println", Args: []string{"result"}},
					{Caller: "ProcessUser", Callee: "GetName"},
					{Caller: "ProcessUser", Callee: "GetAge"},
					{Caller: "ProcessUser", Callee: "Sprintf"},
					{Caller: "FormatString", Callee: "ToUpper"},
					{Caller: "FormatString", Callee: "TrimSpace"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pkgsMetadata := map[string]map[string]*ast.File{}

			exported := packagestest.Export(t, packagestest.GOPATH, tc.src)
			defer exported.Cleanup()
			importPaths := map[string]string{}
			fileToInfo := map[*ast.File]*types.Info{}
			exported.Config.Mode = packages.NeedName | packages.NeedFiles | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedImports
			exported.Config.Fset = fset
			exported.Config.Tests = false

			pkgs, err := packages.Load(exported.Config, "./...")
			if err != nil {
				t.Fatal(err)
			}

			for _, pkg := range pkgs {
				if pkg.PkgPath == "" {
					continue
				}
				pkgsMetadata[pkg.PkgPath] = make(map[string]*ast.File)

				for i, f := range pkg.Syntax {
					if i < len(pkg.GoFiles) {
						pkgsMetadata[pkg.PkgPath][pkg.GoFiles[i]] = f
						fileToInfo[f] = pkg.TypesInfo
						importPaths[pkg.GoFiles[i]] = pkg.PkgPath
					}
				}
			}

			meta := metadata.GenerateMetadata(pkgsMetadata, fileToInfo, importPaths, fset)

			if err := metadata.WriteMetadata(meta, fmt.Sprintf("../spec/tests/%s.yaml", tc.src[0].Name)); err != nil {
				t.Errorf("Failed to write metadata.yaml: %v", err)
			}

			// Validate packages
			for _, expectedPkg := range tc.expected.Packages {
				metaPkg, ok := meta.Packages[expectedPkg.Name]
				if !ok {
					t.Errorf("Package %s not found in metadata", expectedPkg.Name)
					continue
				}

				// Validate files
				for _, expectedFile := range expectedPkg.Files {
					var actualFile *metadata.File
					var filename string
					for fn, f := range metaPkg.Files {
						if strings.HasSuffix(fn, expectedFile.Name) {
							actualFile = f
							filename = fn
							break
						}
					}
					if actualFile == nil {
						t.Errorf("File %s not found in package %s", expectedFile.Name, expectedPkg.Name)
						continue
					}

					// Validate functions
					ok := assert.Equal(t, len(expectedFile.Functions), len(actualFile.Functions),
						"Function count mismatch in %s", filename)
					if !ok {
						return
					}

					for _, expectedFn := range expectedFile.Functions {
						actualFn, ok := actualFile.Functions[expectedFn.Name]
						if !ok {
							t.Errorf("Function %s not found in file %s", expectedFn.Name, filename)
							continue
						}

						// Validate assignments
						if len(expectedFn.Assignments) > 0 {
							ok := assert.Equal(t, len(expectedFn.Assignments), len(actualFn.AssignmentMap),
								"Assignment count mismatch for function %s", expectedFn.Name)
							if !ok {
								return
							}

							for _, expectedAssign := range expectedFn.Assignments {
								_, ok := actualFn.AssignmentMap[expectedAssign]
								assert.Equal(t, ok, true,
									"Assignment %s not found in function %s", expectedAssign, expectedFn.Name)
							}
						}

						// Validate type parameters for generics
						if len(expectedFn.TypeParams) > 0 {
							ok := assert.Equal(t, len(expectedFn.TypeParams), len(actualFn.TypeParams),
								"Type parameter count mismatch for function %s", expectedFn.Name)
							if !ok {
								return
							}

							for i, expectedParam := range expectedFn.TypeParams {
								if i < len(actualFn.TypeParams) {
									assert.Equal(t, expectedParam, actualFn.TypeParams[i],
										"Type parameter mismatch for function %s", expectedFn.Name)
								}
							}
						}

						// Validate return variables
						if expectedFn.ReturnVars > 0 {
							ok := assert.Equal(t, expectedFn.ReturnVars, len(actualFn.ReturnVars),
								"Return variable count mismatch for function %s", expectedFn.Name)
							if !ok {
								return
							}
						}
					}

					// Validate imports
					for _, expectedImport := range expectedFile.Imports {
						expectedImpKey := meta.StringPool.Get(expectedImport.Alias)
						actualImpPath, ok := actualFile.Imports[expectedImpKey]
						ok = assert.Equal(t, true, ok,
							"Import alias %s not found", expectedImport.Alias)
						if !ok {
							return
						}
						if ok {
							ok := assert.Equal(t, expectedImport.Path, meta.StringPool.GetString(actualImpPath),
								"Import path mismatch for alias %s", expectedImport.Alias)
							if !ok {
								return
							}
						}
					}

					// Validate types
					for _, expectedType := range expectedFile.Types {
						actualType, ok := actualFile.Types[expectedType.Name]
						if !ok {
							// Check in package-level types
							actualType, ok = metaPkg.Types[expectedType.Name]
						}
						ok = assert.Equal(t, true, ok,
							"Type %s not found", expectedType.Name)
						if !ok {
							return
						}

						ok = assert.Equal(t, len(expectedType.ImplementedBy), len(actualType.ImplementedBy),
							"Type implemented by count mismatch for %s", expectedType.Name)
						if !ok {
							return
						}
						for i := range expectedType.ImplementedBy {
							ok = assert.Equal(t, expectedType.ImplementedBy[i], meta.StringPool.GetString(actualType.ImplementedBy[i]),
								"Type implemented by mismatch for %s", expectedType.Name)
							if !ok {
								return
							}
						}

						ok = assert.Equal(t, len(expectedType.Implements), len(actualType.Implements),
							"Type implements count mismatch for %s", expectedType.Name)
						if !ok {
							return
						}
						for i := range expectedType.Implements {
							ok = assert.Equal(t, expectedType.Implements[i], meta.StringPool.GetString(actualType.Implements[i]),
								"Type implements mismatch for %s", expectedType.Name)
							if !ok {
								return
							}
						}

						if ok {
							ok = assert.Equal(t, expectedType.Kind, meta.StringPool.GetString(actualType.Kind),
								"Type kind mismatch for %s", expectedType.Name)
							if !ok {
								return
							}

							// Validate fields
							if len(expectedType.Fields) > 0 {
								ok = assert.Equal(t, len(expectedType.Fields), len(actualType.Fields),
									"Field count mismatch for type %s", expectedType.Name)
								if !ok {
									return
								}

								for i, expectedField := range expectedType.Fields {
									if i < len(actualType.Fields) {
										actualField := actualType.Fields[i]
										ok = assert.Equal(t, expectedField.Name, meta.StringPool.GetString(actualField.Name),
											"Field name mismatch for type %s", expectedType.Name)
										if !ok {
											return
										}
										ok = assert.Equal(t, expectedField.Type, meta.StringPool.GetString(actualField.Type),
											"Field type mismatch for type %s", expectedType.Name)
										if !ok {
											return
										}
										if expectedField.Tag != "" {
											ok = assert.Equal(t, expectedField.Tag, meta.StringPool.GetString(actualField.Tag),
												"Field tag mismatch for type %s", expectedType.Name)
											if !ok {
												return
											}
										}
									}
								}
							}

							// Validate methods
							if len(expectedType.Methods) > 0 {
								for _, expectedMethod := range expectedType.Methods {
									found := false
									for _, actualMethod := range actualType.Methods {
										if meta.StringPool.GetString(actualMethod.Name) == expectedMethod.Name {
											found = true
											if expectedMethod.Receiver != "" {
												ok = assert.Equal(t, expectedMethod.Receiver, meta.StringPool.GetString(actualMethod.Receiver),
													"Method receiver mismatch for %s.%s", expectedType.Name, expectedMethod.Name)
												if !ok {
													return
												}
											}
											break
										}
									}
									ok = assert.Equal(t, true, found,
										"Method %s not found for type %s", expectedMethod.Name, expectedType.Name)
									if !ok {
										return
									}
								}
							}
						}
					}

					// Validate variables
					for _, expectedVar := range expectedFile.Variables {
						actualVar, ok := actualFile.Variables[expectedVar.Name]
						ok = assert.Equal(t, true, ok,
							"Variable %s not found", expectedVar.Name)
						if !ok {
							return
						}

						if ok {
							ok = assert.Equal(t, expectedVar.TokType, meta.StringPool.GetString(actualVar.Tok),
								"Variable token type mismatch for %s", expectedVar.Name)
							if !ok {
								return
							}
							if expectedVar.Type != "" {
								ok = assert.Equal(t, expectedVar.Type, meta.StringPool.GetString(actualVar.Type),
									"Variable type mismatch for %s", expectedVar.Name)
							}
							if expectedVar.Value != "" {
								ok = assert.Equal(t, expectedVar.Value, meta.StringPool.GetString(actualVar.Value),
									"Variable value mismatch for %s", expectedVar.Name)
								if !ok {
									return
								}
							}
						}
					}

					// Validate struct instances
					if len(expectedFile.StructInstances) > 0 {
						assert.Equal(t, len(expectedFile.StructInstances), len(actualFile.StructInstances),
							"Struct instance count mismatch in file %s", filename)

						for i, expectedInstance := range expectedFile.StructInstances {
							if i < len(actualFile.StructInstances) {
								actualInstance := actualFile.StructInstances[i]
								ok = assert.Equal(t, expectedInstance.Type, meta.StringPool.GetString(actualInstance.Type),
									"Struct instance type mismatch")
								if !ok {
									return
								}

								for expectedKey, expectedVal := range expectedInstance.Fields {
									keyIdx := meta.StringPool.Get(expectedKey)
									actualVal, ok := actualInstance.Fields[keyIdx]
									ok = assert.Equal(t, true, ok,
										"Struct instance field %s not found", expectedKey)
									if ok {
										ok = assert.Equal(t, expectedVal, meta.StringPool.GetString(actualVal),
											"Struct instance field value mismatch for %s", expectedKey)
										if !ok {
											return
										}
									}
								}
							}
						}
					}
				}
			}

			// Validate call graph
			if len(tc.expected.CallGraph) > 0 {
				actualEdges := make(map[string][]metadata.CallGraphEdge)
				for _, edge := range meta.CallGraph {
					caller := meta.StringPool.GetString(edge.Caller.Name)
					actualEdges[caller] = append(actualEdges[caller], edge)
				}

				for _, expectedEdge := range tc.expected.CallGraph {
					edges, ok := actualEdges[expectedEdge.Caller]
					ok = assert.Equal(t, true, ok,
						"No call graph edges found for caller %s", expectedEdge.Caller)
					if !ok {
						return
					}

					if ok {
						found := false
						for _, actualEdge := range edges {
							calleeName := meta.StringPool.GetString(actualEdge.Callee.Name)
							if calleeName == expectedEdge.Callee {
								found = true

								// Validate arguments if specified
								if len(expectedEdge.Args) > 0 {
									ok = assert.Equal(t, len(expectedEdge.Args), len(actualEdge.Args),
										"Argument count mismatch for call %s->%s", expectedEdge.Caller, expectedEdge.Callee)
									if !ok {
										return
									}
								}

								// Validate parameter count if specified
								if expectedEdge.ParamCount > 0 {
									ok = assert.Equal(t, expectedEdge.ParamCount, len(actualEdge.ParamArgMap),
										"Parameter count mismatch for call %s->%s", expectedEdge.Caller, expectedEdge.Callee)
									if !ok {
										return
									}
								}

								// Validate type parameters if specified
								if len(expectedEdge.TypeParams) > 0 {
									for expectedParam, expectedType := range expectedEdge.TypeParams {
										actualType, ok := actualEdge.TypeParamMap[expectedParam]
										equals := assert.Equal(t, true, ok,
											"Type parameter %s not found for call %s->%s", expectedParam, expectedEdge.Caller, expectedEdge.Callee)
										if !equals {
											return
										}
										if ok {
											equals = assert.Equal(t, expectedType, actualType,
												"Type parameter value mismatch for %s in call %s->%s", expectedParam, expectedEdge.Caller, expectedEdge.Callee)
											if !equals {
												return
											}
										}
									}
								}

								// Validate assignments if specified
								if len(expectedEdge.AssignmentMap) > 0 {
									for expectedParam, expectedType := range expectedEdge.AssignmentMap {
										actualType, ok := actualEdge.AssignmentMap[expectedParam]
										equals := assert.Equal(t, true, ok,
											"Assignment %s not found for call %s->%s", expectedParam, expectedEdge.Caller, expectedEdge.Callee)
										if !equals {
											return
										}
										if ok {
											equals = assert.Equal(t, expectedType, actualType,
												"Assignment value mismatch for %s in call %s->%s", expectedParam, expectedEdge.Caller, expectedEdge.Callee)
											if !equals {
												return
											}
										}
									}
								}
								break
							}
						}
						ok = assert.Equal(t, true, found,
							"Call graph edge %s->%s not found", expectedEdge.Caller, expectedEdge.Callee)
						if !ok {
							return
						}
					}
				}
			}
		})
	}
}

// Helper function to test string pool functionality
func TestStringPool(t *testing.T) {
	pool := metadata.NewStringPool()

	// Test basic functionality
	idx1 := pool.Get("test")
	idx2 := pool.Get("test")
	idx3 := pool.Get("another")

	assert.Equal(t, idx2, idx1)    // Same string should return same index
	assert.NotEqual(t, idx3, idx1) // Different strings should have different indices

	assert.Equal(t, "test", pool.GetString(idx1))
	assert.Equal(t, "another", pool.GetString(idx3))

	// Test empty string
	emptyIdx := pool.Get("")
	assert.Equal(t, -1, emptyIdx)

	// Test invalid index
	assert.Equal(t, "", pool.GetString(-1))
	assert.Equal(t, "", pool.GetString(1000))

	// Test size
	assert.Equal(t, 2, pool.GetSize()) // "test" and "another"
}
