package main

import (
	"fmt"
	"log"

	"github.com/ehabterra/apispec/internal/engine"
)

func main() {
	fmt.Println("Framework Dependency Analysis Demo")
	fmt.Println("==================================")

	// Create engine configuration with framework dependency analysis enabled
	cfg := engine.DefaultEngineConfig()
	cfg.AnalyzeFrameworkDependencies = true
	cfg.AutoIncludeFrameworkPackages = true // Automatically add framework packages to IncludePackages

	// Create engine
	eng := engine.NewEngine(cfg)

	// Generate OpenAPI spec (this will also analyze framework dependencies)
	_, err := eng.GenerateOpenAPI()
	if err != nil {
		log.Fatal(err)
	}

	// Get the metadata
	meta := eng.GetMetadata()
	if meta == nil {
		log.Fatal("No metadata available")
	}

	// Check if framework dependency analysis was performed
	if meta.FrameworkDependencyList == nil {
		fmt.Println("No framework dependency analysis available")
		return
	}

	// Print the dependency list
	meta.FrameworkDependencyList.PrintDependencyList()

	// Get framework packages grouped by type
	frameworkPackages := meta.FrameworkDependencyList.GetFrameworkPackages()

	fmt.Printf("\nFramework Packages by Type:\n")
	fmt.Printf("===========================\n")

	for frameworkType, packages := range frameworkPackages {
		fmt.Printf("\n%s Framework (%d packages):\n", frameworkType, len(packages))
		for _, pkg := range packages {
			fmt.Printf("  Package: %s\n", pkg.PackagePath)
			fmt.Printf("    Files: %d\n", len(pkg.Files))
			fmt.Printf("    Functions: %d\n", len(pkg.Functions))
			fmt.Printf("    Types: %d\n", len(pkg.Types))
			fmt.Printf("    Is Direct: %t\n", pkg.IsDirect)

			if len(pkg.Functions) > 0 {
				fmt.Printf("    Sample Functions: ")
				for i, funcName := range pkg.Functions {
					if i >= 3 { // Show only first 3
						fmt.Printf("...")
						break
					}
					if i > 0 {
						fmt.Printf(", ")
					}
					fmt.Printf("%s", funcName)
				}
				fmt.Printf("\n")
			}
		}
	}

	// Show direct vs indirect packages
	fmt.Printf("\nDirect vs Indirect Packages:\n")
	fmt.Printf("============================\n")

	directPackages := meta.FrameworkDependencyList.GetDirectPackages()
	indirectPackages := meta.FrameworkDependencyList.GetIndirectPackages()

	fmt.Printf("\nDirect Framework Packages (%d):\n", len(directPackages))
	for _, dep := range directPackages {
		fmt.Printf("  %s (%s)\n", dep.PackagePath, dep.FrameworkType)
	}

	fmt.Printf("\nIndirect Framework Packages (%d):\n", len(indirectPackages))
	for _, dep := range indirectPackages {
		fmt.Printf("  %s (%s)\n", dep.PackagePath, dep.FrameworkType)
	}

	// Show statistics
	fmt.Printf("\nStatistics:\n")
	fmt.Printf("===========\n")
	fmt.Printf("Total Packages Analyzed: %d\n", meta.FrameworkDependencyList.TotalPackages)
	fmt.Printf("Direct Framework Packages: %d\n", meta.FrameworkDependencyList.DirectPackages)
	fmt.Printf("Indirect Framework Packages: %d\n", meta.FrameworkDependencyList.IndirectPackages)

	// Show framework type distribution
	fmt.Printf("\nFramework Type Distribution:\n")
	for frameworkType, packages := range meta.FrameworkDependencyList.FrameworkTypes {
		fmt.Printf("  %s: %d packages\n", frameworkType, len(packages))
	}

	// Show the IncludePackages that were automatically populated
	fmt.Printf("\nAuto-Included Packages in IncludePackages:\n")
	fmt.Printf("==========================================\n")
	fmt.Printf("Total IncludePackages: %d\n", len(eng.GetConfig().IncludePackages))

	if len(eng.GetConfig().IncludePackages) > 0 {
		fmt.Println("IncludePackages list:")
		for i, pkg := range eng.GetConfig().IncludePackages {
			fmt.Printf("  %d. %s\n", i+1, pkg)
		}
	}

	fmt.Println("\nDemo completed!")
}
