package spec_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ehabterra/swagen/internal/metadata"
	"github.com/ehabterra/swagen/internal/spec"
)

func TestNewTrackerTree(t *testing.T) {
	// Create tracker with limits
	limits := spec.TrackerLimits{
		MaxNodesPerTree:    100,
		MaxChildrenPerNode: 10,
		MaxArgsPerFunction: 5,
		MaxNestedArgsDepth: 3,
	}

	type Node struct {
		ID                string
		Caller            string
		Callee            string
		CalleeRecvVarName string

		ParamMap  []string
		TypeMap   map[string]string
		Arguments []metadata.CallArgument
		Children  []Node
	}
	type Expected struct {
		RootAssignmentMap map[string][]string
		Roots             []Node
	}

	type testCase struct {
		name         string
		metaFileName string
		expected     Expected
	}

	tests := []testCase{
		{
			name:         "Empty tree",
			metaFileName: "",
			expected:     Expected{},
		},
		{
			name:         "Main Simple tree",
			metaFileName: "tests/main.yaml",
			expected: Expected{
				Roots: []Node{
					{
						ID:     "main.main",
						Caller: "",
						Callee: "",
						Children: []Node{
							{
								ID:     "fmt.Sprintf",
								Caller: "main",
								Callee: "Sprintf",
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: `"%d"`,
									},
									{
										Kind: "ident",
										Name: "x",
										Pkg:  "main",
										Type: "int",
									},
								},
								ParamMap:          []string{"a", "format"},
								CalleeRecvVarName: "z",
								Children: []Node{
									{
										ID:                `"%d"`,
										Caller:            "main",
										Callee:            "Sprintf",
										ParamMap:          []string{"a", "format"},
										CalleeRecvVarName: "z",
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"%d"`,
											},
											{
												Kind: "ident",
												Name: "x",
												Pkg:  "main",
												Type: "int",
											},
										},
									},
									{
										ID:                "int",
										Caller:            "main",
										Callee:            "Sprintf",
										ParamMap:          []string{"a", "format"},
										CalleeRecvVarName: "z",
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"%d"`,
											},
											{
												Kind: "ident",
												Name: "x",
												Pkg:  "main",
												Type: "int",
											},
										},
									},
								},
							},
							{
								ID:       `fmt.Println`,
								Caller:   "main",
								Callee:   "Println",
								ParamMap: []string{"a"},
								Arguments: []metadata.CallArgument{
									{
										Kind: "ident",
										Name: "z",
										Pkg:  "main",
										Type: "string",
									},
								},
								Children: []Node{
									{
										ID:       "string",
										Caller:   "main",
										Callee:   "Println",
										ParamMap: []string{"a"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "z",
												Pkg:  "main",
												Type: "string",
											},
										},
									},
								},
							},
							{
								ID:       `strings.ToUpper`,
								Caller:   "main",
								Callee:   "ToUpper",
								ParamMap: []string{"s"},
								Arguments: []metadata.CallArgument{
									{
										Kind: "ident",
										Name: "y",
										Pkg:  "main",
										Type: "string",
									},
								},
								Children: []Node{
									{
										ID:       "string",
										Caller:   "main",
										Callee:   "ToUpper",
										ParamMap: []string{"s"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "y",
												Pkg:  "main",
												Type: "string",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:         "Struct types with methods and interfaces",
			metaFileName: "tests/example.yaml",
			expected: Expected{
				RootAssignmentMap: map[string][]string{"user": {"NewUser"}},
				Roots: []Node{
					{
						ID:     "example.main",
						Caller: "",
						Callee: "",
						Children: []Node{
							{
								ID:                "example.NewUser",
								Caller:            "main",
								Callee:            "NewUser",
								CalleeRecvVarName: "user",
								ParamMap:          []string{"name", "age"},
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: `"User Name"`,
									},
									{
										Kind:  "literal",
										Value: "40",
									},
								},
								Children: []Node{
									{
										ID:                `"User Name"`,
										Caller:            "main",
										Callee:            "NewUser",
										CalleeRecvVarName: "user",
										ParamMap:          []string{"name", "age"},
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"User Name"`,
											},
											{
												Kind:  "literal",
												Value: "40",
											},
										},
									},
									{
										ID:                "40",
										Caller:            "main",
										Callee:            "NewUser",
										CalleeRecvVarName: "user",
										ParamMap:          []string{"name", "age"},
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"User Name"`,
											},
											{
												Kind:  "literal",
												Value: "40",
											},
										},
									},
									{
										ID:                "example.SetAge@/var/folders/r1/w8tjj19x3nz2gpwll3xntf4r0000gn/T/TestGenerateMetadata_Struct_types_with_methods_and_interfaces3005951015/example/src/example/types.go:34:2",
										Caller:            "NewUser",
										Callee:            "SetAge",
										CalleeRecvVarName: "",
										Children: []Node{
											{
												ID:        "example.age@/var/folders/r1/w8tjj19x3nz2gpwll3xntf4r0000gn/T/TestGenerateMetadata_Struct_types_with_methods_and_interfaces3005951015/example/src/example/types.go:34:11",
												Caller:    "SetAge",
												Callee:    "age",
												ParamMap:  []string{"age"},
												Arguments: []metadata.CallArgument{{Kind: "ident", Name: "age"}},
											},
										},
										ParamMap: []string{"age"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "age",
											},
										},
									},
								},
							},
							{
								ID:       "fmt.Println",
								Caller:   "main",
								Callee:   "Println",
								ParamMap: []string{"a"},
								Arguments: []metadata.CallArgument{
									{
										Kind: "star",
										Name: "user",
										Pkg:  "example",
										Type: "*example.User",
									},
								},
								Children: []Node{
									{
										ID:       "*example.User",
										Caller:   "main",
										Callee:   "Println",
										ParamMap: []string{"a"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "star",
												Name: "user",
												Pkg:  "example",
												Type: "*example.User",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},

		// // Additional test case for interface methods
		// {
		// 	name:         "Interface method calls",
		// 	metaFileName: "tests/interface_methods.yaml",
		// 	expected: Expected{
		// 		Roots: []Node{
		// 			{
		// 				ID:     "example.main",
		// 				Caller: "",
		// 				Callee: "",
		// 				Children: []Node{
		// 					{
		// 						ID:                "example.User.GetName",
		// 						Caller:            "main",
		// 						Callee:            "GetName",
		// 						CalleeRecvVarName: "name",
		// 						ParamMap:          []string{},
		// 						Arguments:         []metadata.CallArgument{},
		// 						Children:          []Node{},
		// 					},
		// 					{
		// 						ID:                "example.User.SetAge",
		// 						Caller:            "main",
		// 						Callee:            "SetAge",
		// 						CalleeRecvVarName: "",
		// 						ParamMap:          []string{"age"},
		// 						Arguments: []metadata.CallArgument{
		// 							{
		// 								Kind:  "literal",
		// 								Value: "25",
		// 							},
		// 						},
		// 						Children: []Node{
		// 							{
		// 								ID:       "25",
		// 								Caller:   "main",
		// 								Callee:   "SetAge",
		// 								ParamMap: []string{"age"},
		// 								Arguments: []metadata.CallArgument{
		// 									{
		// 										Kind:  "literal",
		// 										Value: "25",
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },

		// // Test case for nested function calls with complex arguments
		// {
		// 	name:         "Nested calls with composite literals",
		// 	metaFileName: "tests/nested_composite.yaml",
		// 	expected: Expected{
		// 		Roots: []Node{
		// 			{
		// 				ID:     "example.main",
		// 				Caller: "",
		// 				Callee: "",
		// 				Children: []Node{
		// 					{
		// 						ID:                "example.ProcessUser",
		// 						Caller:            "main",
		// 						Callee:            "ProcessUser",
		// 						CalleeRecvVarName: "result",
		// 						ParamMap:          []string{"user"},
		// 						Arguments: []metadata.CallArgument{
		// 							{
		// 								Kind: "composite_lit",
		// 								Name: "User",
		// 								Pkg:  "example",
		// 								Type: "User",
		// 							},
		// 						},
		// 						Children: []Node{
		// 							{
		// 								ID:                "example.User",
		// 								Caller:            "main",
		// 								Callee:            "ProcessUser",
		// 								CalleeRecvVarName: "result",
		// 								ParamMap:          []string{"user"},
		// 								Arguments: []metadata.CallArgument{
		// 									{
		// 										Kind: "composite_lit",
		// 										Name: "User",
		// 										Pkg:  "example",
		// 										Type: "User",
		// 									},
		// 								},
		// 								Children: []Node{
		// 									{
		// 										ID:       "example.User.Name",
		// 										Caller:   "ProcessUser",
		// 										Callee:   "Name",
		// 										ParamMap: []string{},
		// 										Arguments: []metadata.CallArgument{
		// 											{
		// 												Kind: "ident",
		// 												Name: "UserName",
		// 												Pkg:  "example",
		// 												Type: "string",
		// 											},
		// 										},
		// 									},
		// 									{
		// 										ID:       "example.User.Age",
		// 										Caller:   "ProcessUser",
		// 										Callee:   "Age",
		// 										ParamMap: []string{},
		// 										Arguments: []metadata.CallArgument{
		// 											{
		// 												Kind: "ident",
		// 												Name: "30",
		// 												Pkg:  "example",
		// 												Type: "int",
		// 											},
		// 										},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },

		// // Test case for method chaining
		// {
		// 	name:         "Method chaining",
		// 	metaFileName: "tests/method_chaining.yaml",
		// 	expected: Expected{
		// 		Roots: []Node{
		// 			{
		// 				ID:     "example.main",
		// 				Caller: "",
		// 				Callee: "",
		// 				Children: []Node{
		// 					{
		// 						ID:                "example.NewUser",
		// 						Caller:            "main",
		// 						Callee:            "NewUser",
		// 						CalleeRecvVarName: "user",
		// 						ParamMap:          []string{"name", "age"},
		// 						Arguments: []metadata.CallArgument{
		// 							{
		// 								Kind:  "literal",
		// 								Value: `"John"`,
		// 							},
		// 							{
		// 								Kind:  "literal",
		// 								Value: "30",
		// 							},
		// 						},
		// 						Children: []Node{
		// 							{
		// 								ID:                "example.User.SetAge",
		// 								Caller:            "NewUser",
		// 								Callee:            "SetAge",
		// 								CalleeRecvVarName: "",
		// 								ParamMap:          []string{"age"},
		// 								Arguments: []metadata.CallArgument{
		// 									{
		// 										Kind:  "literal",
		// 										Value: "31",
		// 									},
		// 								},
		// 								Children: []Node{
		// 									{
		// 										ID:                "example.User.GetName",
		// 										Caller:            "SetAge",
		// 										Callee:            "GetName",
		// 										CalleeRecvVarName: "finalName",
		// 										ParamMap:          []string{},
		// 										Arguments:         []metadata.CallArgument{},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },

		// // Test case for interface implementation calls
		// {
		// 	name:         "Interface implementation calls",
		// 	metaFileName: "tests/interface_impl.yaml",
		// 	expected: Expected{
		// 		Roots: []Node{
		// 			{
		// 				ID:     "example.main",
		// 				Caller: "",
		// 				Callee: "",
		// 				Children: []Node{
		// 					{
		// 						ID:                "example.ProcessNamer",
		// 						Caller:            "main",
		// 						Callee:            "ProcessNamer",
		// 						CalleeRecvVarName: "",
		// 						ParamMap:          []string{"n"},
		// 						Arguments: []metadata.CallArgument{
		// 							{
		// 								Kind: "ident",
		// 								Name: "user",
		// 								Pkg:  "example",
		// 								Type: "example.Namer",
		// 							},
		// 						},
		// 						Children: []Node{
		// 							{
		// 								ID:       "example.Namer",
		// 								Caller:   "main",
		// 								Callee:   "ProcessNamer",
		// 								ParamMap: []string{"n"},
		// 								Arguments: []metadata.CallArgument{
		// 									{
		// 										Kind: "ident",
		// 										Name: "user",
		// 										Pkg:  "example",
		// 										Type: "example.Namer",
		// 									},
		// 								},
		// 								Children: []Node{
		// 									{
		// 										ID:                "example.Namer.GetName",
		// 										Caller:            "ProcessNamer",
		// 										Callee:            "GetName",
		// 										CalleeRecvVarName: "name",
		// 										ParamMap:          []string{},
		// 										Arguments:         []metadata.CallArgument{},
		// 									},
		// 								},
		// 							},
		// 						},
		// 					},
		// 				},
		// 			},
		// 		},
		// 	},
		// },
		{
			name:         "Complex call graph with method chains",
			metaFileName: "tests/complex.yaml",
			expected: Expected{
				RootAssignmentMap: map[string][]string{"svc": {"NewService"}, "handler": {"NewHandler"}},
				Roots: []Node{
					{
						ID:     "complex.main",
						Caller: "",
						Callee: "",
						Children: []Node{
							{
								ID:                "complex.NewService",
								Caller:            "main",
								Callee:            "NewService",
								ParamMap:          []string{"name"},
								CalleeRecvVarName: "svc",
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: `"test-service"`,
									},
								},
								Children: []Node{
									{
										ID:                `"test-service"`,
										Caller:            "main",
										Callee:            "NewService",
										ParamMap:          []string{"name"},
										CalleeRecvVarName: "svc",
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"test-service"`,
											},
										},
									},
								},
							},
							{
								ID:                "complex.NewHandler",
								Caller:            "main",
								Callee:            "NewHandler",
								ParamMap:          []string{"svc"},
								CalleeRecvVarName: "handler",
								Arguments: []metadata.CallArgument{
									{
										Kind: "ident",
										Name: "svc",
										Pkg:  "complex",
										Type: "*complex.Service",
									},
								},
								Children: []Node{
									{
										ID:       "svc",
										Caller:   "main",
										Callee:   "NewHandler",
										ParamMap: []string{"svc"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "svc",
												Pkg:  "complex",
												Type: "*complex.Service",
											},
										},
									},
								},
							},
							{
								ID:       "complex.Handle",
								Caller:   "main",
								Callee:   "Handle",
								ParamMap: []string{"input"},
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: `"test-data"`,
									},
								},
								Children: []Node{
									{
										ID:       `"test-data"`,
										Caller:   "main",
										Callee:   "Handle",
										ParamMap: []string{"input"},
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"test-data"`,
											},
										},
									},
									{
										ID:                "complex.GetName",
										Caller:            "Handle",
										Callee:            "GetName",
										ParamMap:          []string{},
										Arguments:         []metadata.CallArgument{},
										CalleeRecvVarName: "name",
									},
									{
										ID:       "complex.Process",
										Caller:   "Handle",
										Callee:   "Process",
										ParamMap: []string{"data"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "input",
												Pkg:  "complex",
												Type: "string",
											},
										},
										CalleeRecvVarName: "processed",
										Children: []Node{
											{
												ID:                "fmt.Sprintf",
												Caller:            "Process",
												Callee:            "Sprintf",
												ParamMap:          []string{"format", "a"},
												CalleeRecvVarName: "result",
												Arguments: []metadata.CallArgument{
													{
														Kind:  "literal",
														Value: `"Processing: %s"`,
													},
													{
														Kind: "ident",
														Name: "data",
														Pkg:  "complex",
														Type: "string",
													},
												},
											},
										},
									},
									{
										ID:       "fmt.Printf",
										Caller:   "Handle",
										Callee:   "Printf",
										ParamMap: []string{"format", "a"},
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"Handler %s: %s\n"`,
											},
											{
												Kind: "ident",
												Name: "name",
												Pkg:  "complex",
												Type: "string",
											},
											{
												Kind: "ident",
												Name: "processed",
												Pkg:  "complex",
												Type: "string",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:         "Generic functions and types",
			metaFileName: "tests/generic.yaml",
			expected: Expected{
				Roots: []Node{
					{
						ID:     "generic.main",
						Caller: "",
						Callee: "",
						Children: []Node{
							{
								ID:                "generic.NewContainer",
								Caller:            "main",
								Callee:            "NewContainer",
								ParamMap:          []string{"value"},
								CalleeRecvVarName: "c",
								TypeMap:           map[string]string{"T": "int"},
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: "42",
									},
								},
							},
							{
								ID:                "generic.Get",
								Caller:            "main",
								Callee:            "Get",
								ParamMap:          []string{},
								CalleeRecvVarName: "val",
								Arguments:         []metadata.CallArgument{},
							},
							{
								ID:       "generic.Set",
								Caller:   "main",
								Callee:   "Set",
								ParamMap: []string{"value"},
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: "100",
									},
								},
							},
							{
								ID:                "generic.Process",
								Caller:            "main",
								Callee:            "Process",
								ParamMap:          []string{"items"},
								CalleeRecvVarName: "result",
								TypeMap:           map[string]string{"T": "string"},
								Arguments: []metadata.CallArgument{
									{
										Kind: "composite_lit",
										Type: "[]string",
									},
								},
								Children: []Node{
									{
										ID:       "len",
										Caller:   "Process",
										Callee:   "len",
										ParamMap: []string{},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "items",
												Pkg:  "generic",
												Type: "[]T",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
		{
			name:         "Multi-package with cross-package dependencies",
			metaFileName: "tests/multipackage.yaml",
			expected: Expected{
				Roots: []Node{
					{
						ID:     "multipackage.main",
						Caller: "",
						Callee: "",
						Children: []Node{
							{
								ID:                "multipackage/models.NewUser",
								Caller:            "main",
								Callee:            "NewUser",
								ParamMap:          []string{"name", "age"},
								CalleeRecvVarName: "user",
								Arguments: []metadata.CallArgument{
									{
										Kind:  "literal",
										Value: `"John"`,
									},
									{
										Kind:  "literal",
										Value: "25",
									},
								},
							},
							{
								ID:                "multipackage/services.NewUserService",
								Caller:            "main",
								Callee:            "NewUserService",
								ParamMap:          []string{},
								CalleeRecvVarName: "service",
								Arguments:         []metadata.CallArgument{},
							},
							{
								ID:                "multipackage/services.ProcessUser",
								Caller:            "main",
								Callee:            "ProcessUser",
								ParamMap:          []string{"user"},
								CalleeRecvVarName: "result",
								Arguments: []metadata.CallArgument{
									{
										Kind: "ident",
										Name: "user",
										Pkg:  "multipackage",
										Type: "*multipackage/models.User",
									},
								},
								Children: []Node{
									{
										ID:                "multipackage/models.GetName",
										Caller:            "ProcessUser",
										Callee:            "GetName",
										ParamMap:          []string{},
										CalleeRecvVarName: "name",
										Arguments:         []metadata.CallArgument{},
									},
									{
										ID:                "multipackage/models.GetAge",
										Caller:            "ProcessUser",
										Callee:            "GetAge",
										ParamMap:          []string{},
										CalleeRecvVarName: "age",
										Arguments:         []metadata.CallArgument{},
									},
									{
										ID:       "fmt.Sprintf",
										Caller:   "ProcessUser",
										Callee:   "Sprintf",
										ParamMap: []string{"format", "a"},
										Arguments: []metadata.CallArgument{
											{
												Kind:  "literal",
												Value: `"%s %s is %d years old"`,
											},
											{
												Kind: "selector",
												Name: "prefix",
												Type: "string",
											},
											{
												Kind: "ident",
												Name: "name",
												Pkg:  "multipackage/services",
												Type: "string",
											},
											{
												Kind: "ident",
												Name: "age",
												Pkg:  "multipackage/services",
												Type: "int",
											},
										},
									},
								},
							},
							{
								ID:       "fmt.Println",
								Caller:   "main",
								Callee:   "Println",
								ParamMap: []string{"a"},
								Arguments: []metadata.CallArgument{
									{
										Kind: "ident",
										Name: "result",
										Pkg:  "multipackage",
										Type: "string",
									},
								},
							},
						},
					},
					{
						ID:     "multipackage/utils.FormatString",
						Caller: "",
						Callee: "",
						Children: []Node{
							{
								ID:       "strings.ToUpper",
								Caller:   "FormatString",
								Callee:   "ToUpper",
								ParamMap: []string{"s"},
								Arguments: []metadata.CallArgument{
									{
										Kind: "call",
										Name: "TrimSpace result",
										Type: "string",
									},
								},
								Children: []Node{
									{
										ID:       "strings.TrimSpace",
										Caller:   "ToUpper",
										Callee:   "TrimSpace",
										ParamMap: []string{"s"},
										Arguments: []metadata.CallArgument{
											{
												Kind: "ident",
												Name: "input",
												Pkg:  "multipackage/utils",
												Type: "string",
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var (
				meta = &metadata.Metadata{}
				err  error
			)

			if tc.metaFileName != "" {
				meta, err = metadata.LoadMetadata(tc.metaFileName)
				if err != nil {
					t.Errorf("Failed to load metadata from file: %s", tc.metaFileName)
				}
			}

			var deepCompare func(t *testing.T, expectedNodes []Node, actualNodes []*spec.TrackerNode)
			deepCompare = func(t *testing.T, expectedNodes []Node, actualNodes []*spec.TrackerNode) {
				ok := assert.Equal(t, len(expectedNodes), len(actualNodes),
					"Nodes should be only %d but found %d", len(expectedNodes), len(actualNodes))
				if !ok {
					return
				}

				for i := range expectedNodes {
					has := strings.HasPrefix(actualNodes[i].Key(), expectedNodes[i].ID+"@")
					if !has {
						has = strings.EqualFold(actualNodes[i].Key(), expectedNodes[i].ID)
					}

					ok := assert.Equal(t, true, has,
						"Node should be %q but found %q", expectedNodes[i].ID, actualNodes[i].Key)
					if !ok {
						return
					}

					if !((expectedNodes[i].Caller == "" && expectedNodes[i].Callee == "") && actualNodes[i].CallGraphEdge == nil) {
						if actualNodes[i].CallGraphEdge == nil {
							t.Errorf("actual node %q edge is empty", expectedNodes[i].ID)
							return
						}

						caller := meta.StringPool.GetString(actualNodes[i].Caller.Name)
						assert.Equal(t, expectedNodes[i].Caller, caller,
							"Node %q should be %q but found %q", expectedNodes[i].ID, expectedNodes[i].Caller, caller)

						callee := meta.StringPool.GetString(actualNodes[i].Callee.Name)
						ok = assert.Equal(t, expectedNodes[i].Callee, callee,
							"Node %q should be %q but found %q", expectedNodes[i].ID, expectedNodes[i].Callee, callee)
						if !ok {
							return
						}

						ok = assert.Equal(t, expectedNodes[i].CalleeRecvVarName, actualNodes[i].CalleeRecvVarName,
							"Node %q should have Var name %q but found %q", expectedNodes[i].ID, expectedNodes[i].CalleeRecvVarName, actualNodes[i].CalleeVarName)
						if !ok {
							return
						}

						ok = assert.Equal(t, len(expectedNodes[i].Arguments), len(actualNodes[i].CallGraphEdge.Args),
							"Node %q args should be %d but found %d", expectedNodes[i].ID, len(expectedNodes[i].Arguments), len(actualNodes[i].CallGraphEdge.Args))
						if !ok {
							return
						}

						for iArg := range expectedNodes[i].Arguments {
							assert.Equal(t, metadata.CallArgToString(expectedNodes[i].Arguments[iArg]), metadata.CallArgToString(actualNodes[i].CallGraphEdge.Args[iArg]),
								"Node %q args should be %d but found %d", expectedNodes[i].ID, metadata.CallArgToString(expectedNodes[i].Arguments[iArg]), metadata.CallArgToString(actualNodes[i].CallGraphEdge.Args[iArg]))
						}

						// Params
						ok = assert.Equal(t, len(expectedNodes[i].ParamMap), len(actualNodes[i].CallGraphEdge.ParamArgMap),
							"Node %q params should be %d but found %d", expectedNodes[i].ID, len(expectedNodes[i].ParamMap), len(actualNodes[i].CallGraphEdge.ParamArgMap))
						if !ok {
							return
						}

						for _, key := range expectedNodes[i].ParamMap {
							_, ok := actualNodes[i].CallGraphEdge.ParamArgMap[key]
							if !ok {
								t.Errorf("actual node %q doesn't have key %q", expectedNodes[i].ID, key)
								return
							}
						}

						// Types
						ok = assert.Equal(t, len(expectedNodes[i].TypeMap), len(actualNodes[i].TypeParams()),
							"Nodes types should be %d but found %d", len(expectedNodes[i].TypeMap), len(actualNodes[i].CallGraphEdge.TypeParamMap))
						if !ok {
							return
						}

						if expectedNodes[i].TypeMap != nil {
							assert.Equal(t, expectedNodes[i].TypeMap, actualNodes[i].TypeParams(),
								"Nodes args should be %d but found %d", expectedNodes[i].TypeMap, actualNodes[i].CallGraphEdge.TypeParamMap)
						}
					}

					deepCompare(t, expectedNodes[i].Children, actualNodes[i].Children)
				}
			}

			tree := spec.NewTrackerTree(meta, limits)

			roots := tree.GetRoots()

			for i := range roots {
				// Root Assignments
				ok := assert.Equal(t, len(tc.expected.RootAssignmentMap), len(roots[i].RootAssignmentMap),
					"Node %q root assignment should be %d but found %d", roots[i].Key, len(tc.expected.RootAssignmentMap), len(roots[i].RootAssignmentMap))
				if !ok {
					return
				}

				for key := range tc.expected.RootAssignmentMap {
					assignments, ok := roots[i].RootAssignmentMap[key]
					if !ok {
						t.Errorf("actual node %q doesn't have key %q", roots[i].Key, key)
						return
					}
					if len(assignments) != len(tc.expected.RootAssignmentMap[key]) {
						t.Errorf("actual node %q doesn't have key %q", roots[i].Key, key)
						return
					}
					for i := range assignments {
						ok = assert.Equal(t, tc.expected.RootAssignmentMap[key][i], assignments[i].CalleeFunc,
							"Node %q assignment should be %q but found %q", roots[i].Key, tc.expected.RootAssignmentMap[key][i], assignments[i].CalleeFunc)
						if !ok {
							return
						}
					}
				}
			}

			deepCompare(t, tc.expected.Roots, roots)
		})
	}
}
