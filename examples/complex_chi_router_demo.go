package main

import (
	"fmt"
	"log"

	"github.com/ehabterra/apispec/internal/engine"
)

// This example demonstrates how APISpec handles complex Chi router structures
// where routes are mounted from handler packages with Routes() methods,
// and top-level groups are presented in separate sub-packages.

func main() {
	fmt.Println("Complex Chi Router Structure Demo")
	fmt.Println("=================================")
	fmt.Println()
	fmt.Println("This demo shows how APISpec can handle complex router patterns like:")
	fmt.Println("- Chi router with mounted routes from handler package Routes() method")
	fmt.Println("- Top-level groups presented in separate sub-packages")
	fmt.Println("- Nested route structures with multiple levels of grouping")
	fmt.Println()

	// Create engine configuration optimized for complex Chi structures
	cfg := engine.DefaultEngineConfig()

	// Enable framework dependency analysis to better understand Chi usage
	cfg.AnalyzeFrameworkDependencies = true

	// Auto-include framework packages for better analysis
	cfg.AutoIncludeFrameworkPackages = true

	// Set reasonable limits for complex projects
	cfg.MaxNodesPerTree = 100000
	cfg.MaxChildrenPerNode = 1000
	cfg.MaxRecursionDepth = 15

	// Include common patterns for Chi router structures
	cfg.IncludePackages = []string{
		"*/handlers", // Handler packages
		"*/routes",   // Route packages
		"*/api",      // API packages
		"*/v1",       // Version packages
		"*/v2",       // Version packages
	}

	// Exclude test files and generated code
	cfg.ExcludeFiles = []string{
		"*_test.go",
		"*_mock.go",
		"*_generated.go",
	}

	fmt.Printf("Configuration for complex Chi router analysis:\n")
	fmt.Printf("- Max nodes per tree: %d\n", cfg.MaxNodesPerTree)
	fmt.Printf("- Max children per node: %d\n", cfg.MaxChildrenPerNode)
	fmt.Printf("- Max recursion depth: %d\n", cfg.MaxRecursionDepth)
	fmt.Printf("- Include packages: %v\n", cfg.IncludePackages)
	fmt.Printf("- Exclude files: %v\n", cfg.ExcludeFiles)
	fmt.Println()

	// Create engine
	eng := engine.NewEngine(cfg)

	// Generate OpenAPI spec
	fmt.Println("Generating OpenAPI specification...")
	openAPISpec, err := eng.GenerateOpenAPI()
	if err != nil {
		log.Fatal(err)
	}

	// Show results
	fmt.Println("✅ OpenAPI specification generated successfully!")
	fmt.Println()

	// Display some statistics
	if openAPISpec != nil && openAPISpec.Paths != nil {
		pathCount := len(openAPISpec.Paths)
		fmt.Printf("📊 Analysis Results:\n")
		fmt.Printf("- Total API paths discovered: %d\n", pathCount)

		// Count different HTTP methods
		methodCounts := make(map[string]int)
		for _, pathItem := range openAPISpec.Paths {
			if pathItem.Get != nil {
				methodCounts["GET"]++
			}
			if pathItem.Post != nil {
				methodCounts["POST"]++
			}
			if pathItem.Put != nil {
				methodCounts["PUT"]++
			}
			if pathItem.Delete != nil {
				methodCounts["DELETE"]++
			}
			if pathItem.Patch != nil {
				methodCounts["PATCH"]++
			}
		}

		fmt.Printf("- HTTP methods found:\n")
		for method, count := range methodCounts {
			fmt.Printf("  %s: %d routes\n", method, count)
		}
	}

	// Show framework analysis results
	meta := eng.GetMetadata()
	if meta != nil && meta.FrameworkDependencyList != nil {
		fmt.Printf("\n🔍 Framework Analysis:\n")
		fmt.Printf("- Total framework packages: %d\n", meta.FrameworkDependencyList.TotalPackages)
		fmt.Printf("- Direct Chi packages: %d\n", meta.FrameworkDependencyList.DirectPackages)
		fmt.Printf("- Indirect packages: %d\n", meta.FrameworkDependencyList.IndirectPackages)

		// Show Chi-specific packages
		chiPackages := meta.FrameworkDependencyList.GetFrameworkPackages()["Chi"]
		if len(chiPackages) > 0 {
			fmt.Printf("- Chi framework packages found:\n")
			for _, pkg := range chiPackages {
				fmt.Printf("  - %s\n", pkg.PackagePath)
			}
		}
	}

	fmt.Println()
	fmt.Println("🎯 Key Features for Complex Router Structures:")
	fmt.Println("- Automatic detection of Chi router patterns")
	fmt.Println("- Support for mounted routes from handler packages")
	fmt.Println("- Recognition of route groups and sub-packages")
	fmt.Println("- Call graph analysis to trace handler functions")
	fmt.Println("- Parameter extraction from path patterns")
	fmt.Println("- Request/response type inference")
	fmt.Println()
	fmt.Println("💡 Tips for Complex Projects:")
	fmt.Println("1. Use --max-nodes flag for large codebases")
	fmt.Println("2. Use --include-package to focus on specific packages")
	fmt.Println("3. Use --diagram to visualize the call graph")
	fmt.Println("4. Use --write-metadata for debugging complex structures")
	fmt.Println("5. Use --analyze-framework-dependencies for better Chi detection")
	fmt.Println()
	fmt.Println("Demo completed! 🚀")
}
