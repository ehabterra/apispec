package metadata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	stringPoolSuffix           = "-string-pool.yaml"
	packagesSuffix             = "-packages.yaml"
	callGraphSuffix            = "-call-graph.yaml"
	filePerm                   = 0644
	errorFailedWriteStringPool = "failed to write string pool: %w"
	errorFailedWritePackages   = "failed to write packages: %w"
	errorFailedWriteCallGraph  = "failed to write call graph: %w"
	errorFailedLoadStringPool  = "failed to load string pool: %w"
	errorFailedLoadPackages    = "failed to load packages: %w"
	errorFailedLoadCallGraph   = "failed to load call graph: %w"
)

// WriteYAML writes any data to a YAML file
func WriteYAML(data interface{}, filename string) error {
	err := os.Remove(filename)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}

	file, err := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, filePerm)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := yaml.NewEncoder(file)
	encoder.SetIndent(2) // This is a good default for readability.

	if err := encoder.Encode(data); err != nil {
		return err
	}

	// It's important to close the encoder to flush any buffered data.
	return encoder.Close()
}

// WriteMetadata writes metadata to a YAML file
func WriteMetadata(metadata *Metadata, filename string) error {
	return WriteYAML(metadata, filename)
}

// WriteSplitMetadata writes metadata split into 3 separate files
func WriteSplitMetadata(metadata *Metadata, baseFilename string) error {
	if metadata == nil {
		return fmt.Errorf("metadata cannot be nil")
	}

	// Extract base path without extension
	basePath := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))

	// Write string pool
	stringPoolFile := basePath + stringPoolSuffix
	if err := WriteYAML(metadata.StringPool, stringPoolFile); err != nil {
		return fmt.Errorf(errorFailedWriteStringPool, err)
	}

	// Write packages
	packagesFile := basePath + packagesSuffix
	if err := WriteYAML(metadata.Packages, packagesFile); err != nil {
		return fmt.Errorf(errorFailedWritePackages, err)
	}

	// Write call graph
	callGraphFile := basePath + callGraphSuffix
	if err := WriteYAML(metadata.CallGraph, callGraphFile); err != nil {
		return fmt.Errorf(errorFailedWriteCallGraph, err)
	}

	return nil
}

// setCallArgumentMeta recursively sets the Meta field for a CallArgument and all its nested CallArguments
func setCallArgumentMeta(arg *CallArgument, meta *Metadata) {
	if arg == nil {
		return
	}
	arg.Meta = meta

	// Set Meta for nested CallArguments
	setCallArgumentMeta(arg.X, meta)
	setCallArgumentMeta(arg.Sel, meta)
	setCallArgumentMeta(arg.Fun, meta)

	// Set Meta for all arguments in Args slice
	for i := range arg.Args {
		setCallArgumentMeta(&arg.Args[i], meta)
	}
}

// setupMetadataReferences sets the Meta field for all CallArgument structs in the metadata
func setupMetadataReferences(metadata *Metadata) {
	// Set Meta field for all CallArgument structs in call graph
	for i := range metadata.CallGraph {
		edge := &metadata.CallGraph[i]
		edge.meta = metadata
		edge.Caller.Meta = metadata
		edge.Callee.Meta = metadata

		// Set Meta for all arguments
		for j := range edge.Args {
			setCallArgumentMeta(&edge.Args[j], metadata)
		}

		// Set Meta for all parameter arguments
		for key, arg := range edge.ParamArgMap {
			setCallArgumentMeta(&arg, metadata)
			edge.ParamArgMap[key] = arg
		}

		// Set Meta for all assignments
		for varName, assignments := range edge.AssignmentMap {
			for j := range assignments {
				setCallArgumentMeta(&assignments[j].Value, metadata)
				setCallArgumentMeta(&assignments[j].Lhs, metadata)
			}
			edge.AssignmentMap[varName] = assignments
		}
	}

	// Set Meta for all assignments and return vars in packages
	for _, pkg := range metadata.Packages {
		for _, file := range pkg.Files {
			for funcName, fn := range file.Functions {
				// Set Meta for function signature
				setCallArgumentMeta(&fn.Signature, metadata)

				// Set Meta for assignments
				for varName, assignments := range fn.AssignmentMap {
					for j := range assignments {
						setCallArgumentMeta(&assignments[j].Value, metadata)
						setCallArgumentMeta(&assignments[j].Lhs, metadata)
					}
					fn.AssignmentMap[varName] = assignments
				}

				// Set Meta for return vars
				for j := range fn.ReturnVars {
					setCallArgumentMeta(&fn.ReturnVars[j], metadata)
				}

				file.Functions[funcName] = fn
			}

			// Set Meta for all methods
			for _, t := range file.Types {
				for i := range t.Methods {
					method := &t.Methods[i]
					// Set Meta for method signature
					setCallArgumentMeta(&method.Signature, metadata)

					for j := range method.ReturnVars {
						setCallArgumentMeta(&method.ReturnVars[j], metadata)
					}
					for varName, assignments := range method.AssignmentMap {
						for j := range assignments {
							setCallArgumentMeta(&assignments[j].Value, metadata)
							setCallArgumentMeta(&assignments[j].Lhs, metadata)
						}
						method.AssignmentMap[varName] = assignments
					}
				}
			}
		}
	}

	// Build the Callers map from the loaded call graph
	metadata.BuildCallGraphMaps()
}

// LoadMetadata loads metadata from a YAML file
func LoadMetadata(filename string) (*Metadata, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var metadata Metadata
	err = yaml.Unmarshal(data, &metadata)
	if err != nil {
		return nil, err
	}

	setupMetadataReferences(&metadata)

	// Process function return types to fill ResolvedType
	metadata.ProcessFunctionReturnTypes()

	return &metadata, nil
}

// LoadSplitMetadata loads metadata from 3 separate files
func LoadSplitMetadata(baseFilename string) (*Metadata, error) {
	// Extract base path without extension
	basePath := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))

	// Load string pool
	stringPoolFile := basePath + stringPoolSuffix
	var stringPool StringPool
	if err := LoadYAML(stringPoolFile, &stringPool); err != nil {
		return nil, fmt.Errorf(errorFailedLoadStringPool, err)
	}

	// Load packages
	packagesFile := basePath + packagesSuffix
	var packages map[string]*Package
	if err := LoadYAML(packagesFile, &packages); err != nil {
		return nil, fmt.Errorf(errorFailedLoadPackages, err)
	}

	// Load call graph
	callGraphFile := basePath + callGraphSuffix
	var callGraph []CallGraphEdge
	if err := LoadYAML(callGraphFile, &callGraph); err != nil {
		return nil, fmt.Errorf(errorFailedLoadCallGraph, err)
	}

	metadata := &Metadata{
		StringPool: &stringPool,
		Packages:   packages,
		CallGraph:  callGraph,
	}

	setupMetadataReferences(metadata)

	// Process function return types to fill ResolvedType
	metadata.ProcessFunctionReturnTypes()

	return metadata, nil
}

// LoadYAML loads data from a YAML file
func LoadYAML(filename string, data interface{}) error {
	fileData, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(fileData, data)
}
