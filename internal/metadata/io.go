package metadata

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// WriteYAML writes any data to a YAML file
func WriteYAML(data interface{}, filename string) error {
	out, err := yaml.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, out, 0644)
}

// WriteMetadata writes metadata to a YAML file
func WriteMetadata(metadata *Metadata, filename string) error {
	return WriteYAML(metadata, filename)
}

// WriteSplitMetadata writes metadata split into 3 separate files
func WriteSplitMetadata(metadata *Metadata, baseFilename string) error {
	// Extract base path without extension
	basePath := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))

	// Write string pool
	stringPoolFile := basePath + "-string-pool.yaml"
	if err := WriteYAML(metadata.StringPool, stringPoolFile); err != nil {
		return fmt.Errorf("failed to write string pool: %w", err)
	}

	// Write packages
	packagesFile := basePath + "-packages.yaml"
	if err := WriteYAML(metadata.Packages, packagesFile); err != nil {
		return fmt.Errorf("failed to write packages: %w", err)
	}

	// Write call graph
	callGraphFile := basePath + "-call-graph.yaml"
	if err := WriteYAML(metadata.CallGraph, callGraphFile); err != nil {
		return fmt.Errorf("failed to write call graph: %w", err)
	}

	return nil
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

	return &metadata, nil
}

// LoadSplitMetadata loads metadata from 3 separate files
func LoadSplitMetadata(baseFilename string) (*Metadata, error) {
	// Extract base path without extension
	basePath := strings.TrimSuffix(baseFilename, filepath.Ext(baseFilename))

	// Load string pool
	stringPoolFile := basePath + "-string-pool.yaml"
	var stringPool StringPool
	if err := LoadYAML(stringPoolFile, &stringPool); err != nil {
		return nil, fmt.Errorf("failed to load string pool: %w", err)
	}

	// Load packages
	packagesFile := basePath + "-packages.yaml"
	var packages map[string]*Package
	if err := LoadYAML(packagesFile, &packages); err != nil {
		return nil, fmt.Errorf("failed to load packages: %w", err)
	}

	// Load call graph
	callGraphFile := basePath + "-call-graph.yaml"
	var callGraph []CallGraphEdge
	if err := LoadYAML(callGraphFile, &callGraph); err != nil {
		return nil, fmt.Errorf("failed to load call graph: %w", err)
	}

	return &Metadata{
		StringPool: &stringPool,
		Packages:   packages,
		CallGraph:  callGraph,
	}, nil
}

// LoadYAML loads data from a YAML file
func LoadYAML(filename string, data interface{}) error {
	fileData, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	return yaml.Unmarshal(fileData, data)
}
