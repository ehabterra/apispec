package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWriteYAML(t *testing.T) {
	// Test writing simple data
	testData := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
		"key3": []string{"a", "b", "c"},
	}

	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "test.yaml")

	// Test successful write
	err := WriteYAML(testData, filename)
	if err != nil {
		t.Errorf("WriteYAML failed: %v", err)
	}

	// Verify file exists and has content
	if _, err := os.Stat(filename); os.IsNotExist(err) {
		t.Error("File was not created")
	}

	// Test writing to existing file (should overwrite)
	err = WriteYAML(testData, filename)
	if err != nil {
		t.Errorf("WriteYAML to existing file failed: %v", err)
	}

	// Test writing to directory that doesn't exist
	nonExistentPath := filepath.Join(tempDir, "nonexistent", "test.yaml")
	err = WriteYAML(testData, nonExistentPath)
	if err == nil {
		t.Error("Expected error when writing to non-existent directory")
	}
}

func TestWriteSplitMetadata(t *testing.T) {
	// Create test metadata
	stringPool := NewStringPool()
	meta := &Metadata{
		StringPool: stringPool,
		Packages: map[string]*Package{
			"main": {
				Files: map[string]*File{
					"main.go": {
						Functions: map[string]*Function{
							"main": {Name: stringPool.Get("main")},
						},
					},
				},
			},
		},
		CallGraph: []CallGraphEdge{
			{
				Caller: Call{Name: stringPool.Get("main")},
				Callee: Call{Name: stringPool.Get("handler")},
			},
		},
	}

	tempDir := t.TempDir()
	baseFilename := filepath.Join(tempDir, "test-metadata")

	// Test successful split write
	err := WriteSplitMetadata(meta, baseFilename)
	if err != nil {
		t.Errorf("WriteSplitMetadata failed: %v", err)
	}

	// Verify all three files were created
	expectedFiles := []string{
		baseFilename + "-string-pool.yaml",
		baseFilename + "-packages.yaml",
		baseFilename + "-call-graph.yaml",
	}

	for _, expectedFile := range expectedFiles {
		if _, err := os.Stat(expectedFile); os.IsNotExist(err) {
			t.Errorf("Expected file %s was not created", expectedFile)
		}
	}

	// Test with nil metadata
	err = WriteSplitMetadata(nil, baseFilename+"-nil")
	if err == nil {
		t.Error("Expected error when writing nil metadata")
	}
}

func TestLoadMetadata(t *testing.T) {
	// Create test metadata and write it
	stringPool := NewStringPool()
	meta := &Metadata{
		StringPool: stringPool,
		Packages: map[string]*Package{
			"main": {
				Files: map[string]*File{
					"main.go": {
						Functions: map[string]*Function{
							"main": {Name: stringPool.Get("main")},
						},
					},
				},
			},
		},
		CallGraph: []CallGraphEdge{
			{
				Caller: Call{Name: stringPool.Get("main")},
				Callee: Call{Name: stringPool.Get("handler")},
			},
		},
	}

	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "test-metadata.yaml")

	// Write metadata first
	err := WriteMetadata(meta, filename)
	if err != nil {
		t.Fatalf("Failed to write test metadata: %v", err)
	}

	// Test loading metadata
	loadedMeta, err := LoadMetadata(filename)
	if err != nil {
		t.Errorf("LoadMetadata failed: %v", err)
	}

	if loadedMeta == nil {
		t.Fatal("Loaded metadata is nil")
		return
	}

	// Verify basic structure
	if loadedMeta.StringPool == nil {
		t.Error("StringPool is nil")
	}
	if loadedMeta.Packages == nil {
		t.Error("Packages is nil")
	}
	if loadedMeta.CallGraph == nil {
		t.Error("CallGraph is nil")
	}

	// Test loading non-existent file
	_, err = LoadMetadata("non-existent-file.yaml")
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}

	// Test loading invalid YAML
	invalidFile := filepath.Join(tempDir, "invalid.yaml")
	err = os.WriteFile(invalidFile, []byte("invalid: yaml: content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid YAML file: %v", err)
	}

	_, err = LoadMetadata(invalidFile)
	if err == nil {
		t.Error("Expected error when loading invalid YAML")
	}
}

func TestLoadSplitMetadata(t *testing.T) {
	// Create test metadata
	stringPool := NewStringPool()
	meta := &Metadata{
		StringPool: stringPool,
		Packages: map[string]*Package{
			"main": {
				Files: map[string]*File{
					"main.go": {
						Functions: map[string]*Function{
							"main": {Name: stringPool.Get("main")},
						},
					},
				},
			},
		},
		CallGraph: []CallGraphEdge{
			{
				Caller: Call{Name: stringPool.Get("main")},
				Callee: Call{Name: stringPool.Get("handler")},
			},
		},
	}

	tempDir := t.TempDir()
	baseFilename := filepath.Join(tempDir, "test-split")

	// Write split metadata first
	err := WriteSplitMetadata(meta, baseFilename)
	if err != nil {
		t.Fatalf("Failed to write split metadata: %v", err)
	}

	// Test loading split metadata
	loadedMeta, err := LoadSplitMetadata(baseFilename)
	if err != nil {
		t.Errorf("LoadSplitMetadata failed: %v", err)
	}

	if loadedMeta == nil {
		t.Fatal("Loaded metadata is nil")
		return
	}

	// Verify basic structure
	if loadedMeta.StringPool == nil {
		t.Error("StringPool is nil")
	}
	if loadedMeta.Packages == nil {
		t.Error("Packages is nil")
	}
	if loadedMeta.CallGraph == nil {
		t.Error("CallGraph is nil")
	}

	// Test loading with missing files
	_, err = LoadSplitMetadata("non-existent-base")
	if err == nil {
		t.Error("Expected error when loading non-existent split metadata")
	}
}

func TestLoadYAML(t *testing.T) {
	// Test loading valid YAML
	testData := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}

	tempDir := t.TempDir()
	filename := filepath.Join(tempDir, "test-load.yaml")

	err := WriteYAML(testData, filename)
	if err != nil {
		t.Fatalf("Failed to write test YAML: %v", err)
	}

	var loadedData map[string]interface{}
	err = LoadYAML(filename, &loadedData)
	if err != nil {
		t.Errorf("LoadYAML failed: %v", err)
	}

	// Verify loaded data
	if loadedData["key1"] != "value1" {
		t.Errorf("Expected key1 to be 'value1', got %v", loadedData["key1"])
	}
	if loadedData["key2"] != 42 {
		t.Errorf("Expected key2 to be 42, got %v", loadedData["key2"])
	}

	// Test loading non-existent file
	err = LoadYAML("non-existent.yaml", &loadedData)
	if err == nil {
		t.Error("Expected error when loading non-existent file")
	}

	// Test loading invalid YAML
	invalidFile := filepath.Join(tempDir, "invalid-load.yaml")
	err = os.WriteFile(invalidFile, []byte("invalid: yaml: content"), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid YAML file: %v", err)
	}

	err = LoadYAML(invalidFile, &loadedData)
	if err == nil {
		t.Error("Expected error when loading invalid YAML")
	}
}

func TestSetupMetadataReferences(t *testing.T) {
	// Create test metadata with call arguments
	stringPool := NewStringPool()
	meta := &Metadata{
		StringPool: stringPool,
		CallGraph: []CallGraphEdge{
			{
				Caller: Call{Name: stringPool.Get("main")},
				Callee: Call{Name: stringPool.Get("handler")},
				Args: []*CallArgument{
					{
						Kind: stringPool.Get("ident"),
						Name: stringPool.Get("arg1"),
						Type: stringPool.Get("string"),
					},
				},
				ParamArgMap: map[string]CallArgument{
					"param1": {
						Kind: stringPool.Get("ident"),
						Name: stringPool.Get("param1"),
						Type: stringPool.Get("int"),
					},
				},
				AssignmentMap: map[string][]Assignment{
					"var1": {
						{
							Lhs:   CallArgument{Kind: stringPool.Get("ident"), Name: stringPool.Get("var1")},
							Value: CallArgument{Kind: stringPool.Get("literal"), Value: stringPool.Get("value1")},
						},
					},
				},
			},
		},
		Packages: map[string]*Package{
			"main": {
				Files: map[string]*File{
					"main.go": {
						Functions: map[string]*Function{
							"main": {
								Name: stringPool.Get("main"),
								Signature: CallArgument{
									Kind: stringPool.Get("ident"),
									Name: stringPool.Get("main"),
								},
								ReturnVars: []CallArgument{
									{
										Kind: stringPool.Get("ident"),
										Name: stringPool.Get("result"),
									},
								},
								AssignmentMap: map[string][]Assignment{
									"funcVar": {
										{
											Lhs:   CallArgument{Kind: stringPool.Get("ident"), Name: stringPool.Get("funcVar")},
											Value: CallArgument{Kind: stringPool.Get("literal"), Value: stringPool.Get("funcValue")},
										},
									},
								},
							},
						},
						Types: map[string]*Type{
							"Handler": {
								Name: stringPool.Get("Handler"),
								Methods: []Method{
									{
										Name: stringPool.Get("Handle"),
										Signature: CallArgument{
											Kind: stringPool.Get("ident"),
											Name: stringPool.Get("Handle"),
										},
										ReturnVars: []CallArgument{
											{
												Kind: stringPool.Get("ident"),
												Name: stringPool.Get("methodResult"),
											},
										},
										AssignmentMap: map[string][]Assignment{
											"methodVar": {
												{
													Lhs:   CallArgument{Kind: stringPool.Get("ident"), Name: stringPool.Get("methodVar")},
													Value: CallArgument{Kind: stringPool.Get("literal"), Value: stringPool.Get("methodValue")},
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
		},
	}

	// Test setupMetadataReferences
	setupMetadataReferences(meta)

	// Verify that Meta fields are set
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		if edge.meta != meta {
			t.Error("Edge meta not set correctly")
		}
		if edge.Caller.Meta != meta {
			t.Error("Caller meta not set correctly")
		}
		if edge.Callee.Meta != meta {
			t.Error("Callee meta not set correctly")
		}

		// Check arguments
		for j := range edge.Args {
			if edge.Args[j].Meta != meta {
				t.Error("Argument meta not set correctly")
			}
		}

		// Check parameter arguments
		for _, arg := range edge.ParamArgMap {
			if arg.Meta != meta {
				t.Error("Parameter argument meta not set correctly")
			}
		}

		// Check assignments
		for _, assignments := range edge.AssignmentMap {
			for _, assignment := range assignments {
				if assignment.Value.Meta != meta {
					t.Error("Assignment value meta not set correctly")
				}
				if assignment.Lhs.Meta != meta {
					t.Error("Assignment lhs meta not set correctly")
				}
			}
		}
	}

	// Check package functions
	for _, pkg := range meta.Packages {
		for _, file := range pkg.Files {
			for _, fn := range file.Functions {
				if fn.Signature.Meta != meta {
					t.Error("Function signature meta not set correctly")
				}

				for _, retVar := range fn.ReturnVars {
					if retVar.Meta != meta {
						t.Error("Function return var meta not set correctly")
					}
				}

				for _, assignments := range fn.AssignmentMap {
					for _, assignment := range assignments {
						if assignment.Value.Meta != meta {
							t.Error("Function assignment value meta not set correctly")
						}
						if assignment.Lhs.Meta != meta {
							t.Error("Function assignment lhs meta not set correctly")
						}
					}
				}
			}

			// Check types and methods
			for _, typ := range file.Types {
				for _, method := range typ.Methods {
					if method.Signature.Meta != meta {
						t.Error("Method signature meta not set correctly")
					}

					for _, retVar := range method.ReturnVars {
						if retVar.Meta != meta {
							t.Error("Method return var meta not set correctly")
						}
					}

					for _, assignments := range method.AssignmentMap {
						for _, assignment := range assignments {
							if assignment.Value.Meta != meta {
								t.Error("Method assignment value meta not set correctly")
							}
							if assignment.Lhs.Meta != meta {
								t.Error("Method assignment lhs meta not set correctly")
							}
						}
					}
				}
			}
		}
	}
}

func TestSetCallArgumentMeta(t *testing.T) {
	// Test with nil argument
	setCallArgumentMeta(nil, &Metadata{})

	// Test with simple argument
	meta := &Metadata{}
	stringPool := NewStringPool()
	meta.StringPool = stringPool

	arg := &CallArgument{
		Kind: stringPool.Get("ident"),
		Name: stringPool.Get("test"),
	}

	setCallArgumentMeta(arg, meta)
	if arg.Meta != meta {
		t.Error("Meta not set on simple argument")
	}

	// Test with nested arguments
	nestedArg := &CallArgument{
		Kind: stringPool.Get("call"),
		Name: stringPool.Get("nested"),
		X: &CallArgument{
			Kind: stringPool.Get("ident"),
			Name: stringPool.Get("x"),
		},
		Sel: &CallArgument{
			Kind: stringPool.Get("ident"),
			Name: stringPool.Get("sel"),
		},
		Fun: &CallArgument{
			Kind: stringPool.Get("ident"),
			Name: stringPool.Get("fun"),
		},
		Args: []*CallArgument{
			{
				Kind: stringPool.Get("ident"),
				Name: stringPool.Get("arg1"),
			},
			{
				Kind: stringPool.Get("ident"),
				Name: stringPool.Get("arg2"),
			},
		},
	}

	setCallArgumentMeta(nestedArg, meta)

	// Verify all nested arguments have meta set
	if nestedArg.Meta != meta {
		t.Error("Root argument meta not set")
	}
	if nestedArg.X.Meta != meta {
		t.Error("X argument meta not set")
	}
	if nestedArg.Sel.Meta != meta {
		t.Error("Sel argument meta not set")
	}
	if nestedArg.Fun.Meta != meta {
		t.Error("Fun argument meta not set")
	}

	for i := range nestedArg.Args {
		if nestedArg.Args[i].Meta != meta {
			t.Errorf("Args[%d] meta not set", i)
		}
	}
}
