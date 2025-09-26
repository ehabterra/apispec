package main

import (
	"fmt"
	"log"

	"github.com/ehabterra/apispec/internal/engine"
)

func main() {
	fmt.Println("Auto-Include Framework Packages Demo")
	fmt.Println("====================================")

	// Create engine configuration
	cfg := engine.DefaultEngineConfig()

	// Enable framework dependency analysis
	cfg.AnalyzeFrameworkDependencies = true

	// Enable auto-inclusion of framework packages
	cfg.AutoIncludeFrameworkPackages = true

	// You can also manually add some packages to IncludePackages
	cfg.IncludePackages = []string{
		"github.com/yourproject/shared", // This will be preserved
	}

	fmt.Printf("Initial IncludePackages: %v\n", cfg.IncludePackages)

	// Create engine
	eng := engine.NewEngine(cfg)

	// Generate OpenAPI spec (analyzes dependencies, auto-includes framework packages, and filters metadata)
	_, err := eng.GenerateOpenAPI()
	if err != nil {
		log.Fatal(err)
	}

	// Show the final IncludePackages configuration
	fmt.Printf("\nFinal IncludePackages after auto-inclusion:\n")
	fmt.Printf("==========================================\n")

	finalConfig := eng.GetConfig()
	fmt.Printf("Total IncludePackages: %d\n", len(finalConfig.IncludePackages))

	if len(finalConfig.IncludePackages) > 0 {
		fmt.Println("IncludePackages list:")
		for i, pkg := range finalConfig.IncludePackages {
			fmt.Printf("  %d. %s\n", i+1, pkg)
		}
	}

	// Show framework dependency analysis results
	meta := eng.GetMetadata()
	if meta != nil && meta.FrameworkDependencyList != nil {
		fmt.Printf("\nFramework Analysis Results:\n")
		fmt.Printf("===========================\n")
		fmt.Printf("Total Framework Packages: %d\n", meta.FrameworkDependencyList.TotalPackages)
		fmt.Printf("Direct Framework Packages: %d\n", meta.FrameworkDependencyList.DirectPackages)
		fmt.Printf("Indirect Framework Packages: %d\n", meta.FrameworkDependencyList.IndirectPackages)
	}

	// Show which packages were auto-included
	fmt.Printf("\nAuto-included framework packages:\n")
	for _, dep := range meta.FrameworkDependencyList.AllPackages {
		frameworkType := dep.FrameworkType
		if dep.IsDirect {
			frameworkType += " (direct)"
		} else {
			frameworkType += " (indirect)"
		}
		fmt.Printf("  - %s (%s)\n", dep.PackagePath, frameworkType)
	}

	// Show imported packages separately
	importedPackages := meta.FrameworkDependencyList.GetImportedPackages()
	if len(importedPackages) > 0 {
		fmt.Printf("\nImported packages by framework packages:\n")
		for _, dep := range importedPackages {
			fmt.Printf("  - %s (imported by framework packages)\n", dep.PackagePath)
		}
	}

	fmt.Println("\nDemo completed!")
	fmt.Println("\nThis demonstrates how framework packages are automatically")
	fmt.Println("added to IncludePackages, allowing you to focus OpenAPI")
	fmt.Println("generation on framework-related code.")
	fmt.Println("\nOPTIMIZATION: When AutoIncludeFrameworkPackages=true,")
	fmt.Println("metadata generation only works on framework packages,")
	fmt.Println("making the process much more efficient!")
}
