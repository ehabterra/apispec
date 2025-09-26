package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

func main() {
	// Example: Generate Cytoscape visualization from call graph metadata
	fmt.Println("Call Graph Cytoscape Demo")
	fmt.Println("=========================")

	// Create a simple metadata with call graph
	meta := createSampleMetadata()

	// Generate HTML visualization using call graph data
	outputPath := "call_graph_diagram.html"
	err := spec.GenerateCallGraphCytoscapeHTML(meta, outputPath)
	if err != nil {
		log.Fatalf("Failed to generate call graph HTML: %v", err)
	}

	fmt.Printf("✅ Generated call graph visualization: %s\n", outputPath)

	// Also generate JSON export
	jsonPath := "call_graph_data.json"
	err = spec.ExportCallGraphCytoscapeJSON(meta, jsonPath)
	if err != nil {
		log.Fatalf("Failed to export call graph JSON: %v", err)
	}

	fmt.Printf("✅ Exported call graph data: %s\n", jsonPath)

	// Get absolute paths for easy access
	absHTML, _ := filepath.Abs(outputPath)
	absJSON, _ := filepath.Abs(jsonPath)

	fmt.Printf("\n📁 Files created:\n")
	fmt.Printf("   HTML: %s\n", absHTML)
	fmt.Printf("   JSON: %s\n", absJSON)

	fmt.Printf("\n🌐 Open the HTML file in your browser to see the interactive call graph!\n")
	fmt.Printf("   The visualization shows:\n")
	fmt.Printf("   • Function names as node labels\n")
	fmt.Printf("   • Click nodes to see detailed popup information:\n")
	fmt.Printf("     - Package information\n")
	fmt.Printf("     - Call paths\n")
	fmt.Printf("     - Parameter types\n")
	fmt.Printf("     - Passed parameters\n")
	fmt.Printf("     - Generic type information\n")
	fmt.Printf("     - Position information\n")
}

func createSampleMetadata() *metadata.Metadata {
	// Create a string pool
	stringPool := metadata.NewStringPool()

	// Add strings to the pool
	mainPkg := stringPool.Get("main")
	mainFunc := stringPool.Get("main")
	fooFunc := stringPool.Get("foo")
	barFunc := stringPool.Get("bar")
	bazFunc := stringPool.Get("baz")
	stringType := stringPool.Get("string")
	intType := stringPool.Get("int")

	// Create call graph edges
	callGraph := []metadata.CallGraphEdge{
		{
			Caller: metadata.Call{
				Name:     mainFunc,
				Pkg:      mainPkg,
				Position: stringPool.Get("main.go:5"),
				Meta:     nil, // Will be set later
			},
			Callee: metadata.Call{
				Name:     fooFunc,
				Pkg:      mainPkg,
				Position: stringPool.Get("main.go:10"),
				Meta:     nil, // Will be set later
			},
			Position: stringPool.Get("main.go:5"),
			Args: []metadata.CallArgument{
				{
					Name:     stringPool.Get("hello"),
					Type:     stringType,
					Position: stringPool.Get("main.go:5"),
					Meta:     nil, // Will be set later
				},
			},
			ParamArgMap: map[string]metadata.CallArgument{
				"msg": {
					Name:     stringPool.Get("hello"),
					Type:     stringType,
					Position: stringPool.Get("main.go:5"),
					Meta:     nil, // Will be set later
				},
			},
			TypeParamMap: map[string]string{
				"T": "string",
			},
		},
		{
			Caller: metadata.Call{
				Name:     fooFunc,
				Pkg:      mainPkg,
				Position: stringPool.Get("main.go:10"),
				Meta:     nil, // Will be set later
			},
			Callee: metadata.Call{
				Name:     barFunc,
				Pkg:      mainPkg,
				Position: stringPool.Get("main.go:15"),
				Meta:     nil, // Will be set later
			},
			Position: stringPool.Get("main.go:10"),
			Args: []metadata.CallArgument{
				{
					Name:     stringPool.Get("42"),
					Type:     intType,
					Position: stringPool.Get("main.go:10"),
					Meta:     nil, // Will be set later
				},
			},
			ParamArgMap: map[string]metadata.CallArgument{
				"count": {
					Name:     stringPool.Get("42"),
					Type:     intType,
					Position: stringPool.Get("main.go:10"),
					Meta:     nil, // Will be set later
				},
			},
		},
		{
			Caller: metadata.Call{
				Name:     barFunc,
				Pkg:      mainPkg,
				Position: stringPool.Get("main.go:15"),
				Meta:     nil, // Will be set later
			},
			Callee: metadata.Call{
				Name:     bazFunc,
				Pkg:      mainPkg,
				Position: stringPool.Get("main.go:20"),
				Meta:     nil, // Will be set later
			},
			Position: stringPool.Get("main.go:15"),
			Args: []metadata.CallArgument{
				{
					Name:     stringPool.Get("true"),
					Type:     stringPool.Get("bool"),
					Position: stringPool.Get("main.go:15"),
					Meta:     nil, // Will be set later
				},
			},
			ParamArgMap: map[string]metadata.CallArgument{
				"enabled": {
					Name:     stringPool.Get("true"),
					Type:     stringPool.Get("bool"),
					Position: stringPool.Get("main.go:15"),
					Meta:     nil, // Will be set later
				},
			},
		},
	}

	// Create metadata
	meta := &metadata.Metadata{
		StringPool: stringPool,
		CallGraph:  callGraph,
	}

	// Set metadata references
	for i := range meta.CallGraph {
		edge := &meta.CallGraph[i]
		edge.Caller.Meta = meta
		edge.Callee.Meta = meta

		// Set Meta for all arguments
		for j := range edge.Args {
			edge.Args[j].Meta = meta
		}

		// Set Meta for all parameter arguments
		for key, arg := range edge.ParamArgMap {
			arg.Meta = meta
			edge.ParamArgMap[key] = arg
		}
	}

	// Build call graph maps
	meta.BuildCallGraphMaps()

	return meta
}
