// Copyright 2025 Ehab Terra
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package main

import (
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

//go:embed server_ui.html
var serverUITemplate embed.FS

// Version info - can be injected at build time via -ldflags or detected at runtime
var (
	Version   = "0.0.1" // Default version, overridden by -ldflags or runtime detection
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = "unknown"
)

// detectVersionInfo attempts to detect version information at runtime
func detectVersionInfo() {
	// If version info was already injected via -ldflags, don't override it
	if Version != "0.0.1" {
		return
	}

	// Try to get build info from runtime/debug
	if info, ok := debug.ReadBuildInfo(); ok {
		// Always get Go version from build info
		if info.GoVersion != "" {
			GoVersion = info.GoVersion
		}

		// Get version from build info (usually the module version or VCS tag)
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			Version = info.Main.Version
		}

		// Extract commit, build time, and other VCS info from build settings
		hasVCSInfo := false
		isModified := false
		for _, setting := range info.Settings {
			switch setting.Key {
			case "vcs.revision":
				hasVCSInfo = true
				if len(setting.Value) >= 7 {
					Commit = setting.Value[:7] // Short commit hash
				} else {
					Commit = setting.Value
				}
			case "vcs.time":
				hasVCSInfo = true
				BuildDate = setting.Value
			case "vcs.modified":
				if setting.Value == "true" {
					isModified = true
				}
			}
		}

		// Add dirty flag if modified (but only if we don't already have it)
		if isModified && !strings.Contains(Version, "+dirty") {
			Version += "+dirty"
		}

		// If we have VCS info but no version, we're likely in development
		if hasVCSInfo && Version == "0.0.1" {
			Version = "dev"
		}
	}
}

func printVersion() {
	// Detect version info if not already set via -ldflags
	detectVersionInfo()

	fmt.Printf("apidiag version: %s\n", Version)
	fmt.Printf("Commit: %s\n", Commit)
	fmt.Printf("Build date: %s\n", BuildDate)
	fmt.Printf("Go version: %s\n", GoVersion)
}

// matchesFunctionName performs case-insensitive substring matching for function names
func matchesFunctionName(functionName, searchTerm string) bool {
	if searchTerm == "" {
		return false
	}
	return strings.Contains(strings.ToLower(functionName), strings.ToLower(strings.TrimSpace(searchTerm)))
}

// ServerConfig holds configuration for the API diagram server
type ServerConfig struct {
	Port                         int
	Host                         string
	InputDir                     string
	PageSize                     int
	MaxDepth                     int
	EnableCORS                   bool
	CacheTimeout                 time.Duration
	StaticDir                    string
	Verbose                      bool
	AnalyzeFrameworkDependencies bool
	AutoIncludeFrameworkPackages bool
	AutoExcludeTests             bool
	AutoExcludeMocks             bool
	ShowVersion                  bool
	DiagramType                  string
}

// DiagramServer handles HTTP requests for paginated diagram data
type DiagramServer struct {
	config   *ServerConfig
	metadata *metadata.Metadata
	// cache for paginated Cytoscape datasets to avoid re-generating in multiple places
	cache    map[string]*spec.PaginatedCytoscapeData
	lastLoad time.Time
	// cache for full Cytoscape datasets to avoid re-generating in multiple places
	dataCache map[string]*spec.CytoscapeData
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Nodes       []spec.CytoscapeNode `json:"nodes"`
	Edges       []spec.CytoscapeEdge `json:"edges"`
	TotalNodes  int                  `json:"total_nodes"`
	TotalEdges  int                  `json:"total_edges"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"page_size"`
	HasMore     bool                 `json:"has_more"`
	LoadTime    time.Duration        `json:"load_time_ms"`
	DiagramType string               `json:"diagram_type"` // "call-graph" or "tracker-tree"
}

// ErrorResponse represents an error response
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

func main() {
	config := parseFlags()

	// Handle version flag early
	if config.ShowVersion {
		printVersion()
		os.Exit(0)
	}

	// Create server
	server := NewDiagramServer(config)

	// Load metadata
	if err := server.LoadMetadata(); err != nil {
		log.Fatalf("Failed to load metadata: %v", err)
	}

	// Setup routes
	server.SetupRoutes()

	// Start server
	addr := fmt.Sprintf("%s:%d", config.Host, config.Port)
	log.Printf("🚀 API Diagram server starting on http://%s", addr)
	if config.Verbose {
		log.Printf("📊 Serving %s diagrams for: %s", config.DiagramType, config.InputDir)
		log.Printf("⚙️  Page size: %d, Max depth: %d", config.PageSize, config.MaxDepth)
	}

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func parseFlags() *ServerConfig {
	config := &ServerConfig{}

	// Version flag
	flag.BoolVar(&config.ShowVersion, "version", false, "Show version information")
	flag.BoolVar(&config.ShowVersion, "V", false, "Shorthand for --version")

	flag.IntVar(&config.Port, "port", 8080, "Server port")
	flag.StringVar(&config.Host, "host", "localhost", "Server host")
	flag.StringVar(&config.InputDir, "dir", ".", "Input directory containing Go source files")
	flag.IntVar(&config.PageSize, "page-size", 100, "Default page size for pagination")
	flag.IntVar(&config.MaxDepth, "max-depth", 3, "Maximum call graph depth")
	flag.BoolVar(&config.EnableCORS, "cors", true, "Enable CORS headers")
	flag.DurationVar(&config.CacheTimeout, "cache-timeout", 5*time.Minute, "Cache timeout for metadata")
	flag.StringVar(&config.StaticDir, "static", "", "Directory to serve static files from")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")
	flag.BoolVar(&config.Verbose, "v", false, "Shorthand for --verbose")

	flag.BoolVar(&config.AnalyzeFrameworkDependencies, "analyze-framework-dependencies", false, "Analyze framework dependencies")
	flag.BoolVar(&config.AnalyzeFrameworkDependencies, "afd", false, "Shorthand for --analyze-framework-dependencies")

	flag.BoolVar(&config.AutoIncludeFrameworkPackages, "auto-include-framework-packages", false, "Auto-include framework packages")
	flag.BoolVar(&config.AutoIncludeFrameworkPackages, "aifp", false, "Shorthand for --auto-include-framework-packages")

	flag.BoolVar(&config.AutoExcludeTests, "auto-exclude-tests", false, "Auto-exclude test files")
	flag.BoolVar(&config.AutoExcludeTests, "aet", false, "Shorthand for --auto-exclude-tests")

	flag.BoolVar(&config.AutoExcludeMocks, "auto-exclude-mocks", false, "Auto-exclude mock files")
	flag.BoolVar(&config.AutoExcludeMocks, "aem", false, "Shorthand for --auto-exclude-mocks")

	flag.StringVar(&config.DiagramType, "diagram-type", "call-graph", "Diagram type: 'call-graph' or 'tracker-tree'")
	flag.StringVar(&config.DiagramType, "dt", "call-graph", "Shorthand for --diagram-type")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "APISpec API Diagram Server - Serves paginated call graph diagrams\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --port 8080\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --page-size 50 --max-depth 2\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --diagram-type tracker-tree\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --static ./public --cors\n", os.Args[0])
	}

	flag.Parse()

	// Validate page size
	if config.PageSize < 10 {
		config.PageSize = 10
	} else if config.PageSize > 1000 {
		config.PageSize = 1000
	}

	// Validate max depth
	if config.MaxDepth < 1 {
		config.MaxDepth = 1
	} else if config.MaxDepth > 10 {
		config.MaxDepth = 10
	}

	// Validate diagram type
	if config.DiagramType != "call-graph" && config.DiagramType != "tracker-tree" {
		config.DiagramType = "call-graph" // Default to call-graph if invalid
	}

	return config
}

// NewDiagramServer creates a new diagram server
func NewDiagramServer(config *ServerConfig) *DiagramServer {
	return &DiagramServer{
		config:    config,
		cache:     make(map[string]*spec.PaginatedCytoscapeData),
		dataCache: make(map[string]*spec.CytoscapeData),
	}
}

// LoadMetadata loads and analyzes the Go project
func (s *DiagramServer) LoadMetadata() error {
	log.Printf("📁 Analyzing project: %s", s.config.InputDir)

	// Create engine configuration
	engineConfig := &engine.EngineConfig{
		Verbose:                      s.config.Verbose,
		InputDir:                     s.config.InputDir,
		MaxNodesPerTree:              50000,
		MaxChildrenPerNode:           500,
		MaxArgsPerFunction:           100,
		MaxNestedArgsDepth:           100,
		MaxRecursionDepth:            s.config.MaxDepth,
		SkipCGOPackages:              true,
		AnalyzeFrameworkDependencies: s.config.AnalyzeFrameworkDependencies,
		AutoIncludeFrameworkPackages: s.config.AutoIncludeFrameworkPackages,
		AutoExcludeTests:             s.config.AutoExcludeTests, // Enable auto-exclude for test files
		AutoExcludeMocks:             s.config.AutoExcludeMocks, // Enable auto-exclude for mock files
	}

	// Create engine and generate metadata only (no OpenAPI spec needed)
	genEngine := engine.NewEngine(engineConfig)
	meta, err := genEngine.GenerateMetadataOnly()
	if err != nil {
		return fmt.Errorf("failed to generate metadata: %w", err)
	}

	// Store metadata
	s.metadata = meta

	s.lastLoad = time.Now()

	log.Printf("✅ Metadata loaded successfully")
	if s.config.Verbose {
		log.Printf("📊 Total packages: %d", len(s.metadata.Packages))
		log.Printf("📊 Total call graph edges: %d", len(s.metadata.CallGraph))
	}

	return nil
}

// SetupRoutes sets up HTTP routes
func (s *DiagramServer) SetupRoutes() {
	// API routes
	http.HandleFunc("/api/diagram", s.handleDiagram)
	http.HandleFunc("/api/diagram/page", s.handlePaginatedDiagram)
	http.HandleFunc("/api/diagram/packages", s.handlePackageHierarchy)
	http.HandleFunc("/api/diagram/by-packages", s.handlePackageBasedDiagram)
	http.HandleFunc("/api/diagram/stats", s.handleStats)
	http.HandleFunc("/api/diagram/refresh", s.handleRefresh)
	http.HandleFunc("/api/diagram/export", s.handleExport)

	// Health check
	http.HandleFunc("/health", s.handleHealth)

	// Serve static files if configured
	if s.config.StaticDir != "" {
		http.Handle("/", http.FileServer(http.Dir(s.config.StaticDir)))
	} else {
		// Default route with basic HTML interface
		http.HandleFunc("/", s.handleIndex)
	}
}

// handleIndex serves the embedded interactive UI
func (s *DiagramServer) handleIndex(w http.ResponseWriter, r *http.Request) {
	// Read the embedded HTML template
	templateBytes, err := serverUITemplate.ReadFile("server_ui.html")
	if err != nil {
		s.writeError(w, "Failed to load UI template", http.StatusInternalServerError)
		return
	}

	// Replace the placeholder with actual server URL
	htmlTemplate := string(templateBytes)
	serverURL := fmt.Sprintf("http://%s:%d", s.config.Host, s.config.Port)
	htmlContent := strings.Replace(htmlTemplate, "%s", serverURL, 1)

	s.writeResponse(w, htmlContent, "text/html")
}

// handleDiagram serves complete diagram data
func (s *DiagramServer) handleDiagram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	// Generate diagram data based on configured type (cached)
	data := s.getAllData(s.config.DiagramType, false)

	loadTime := time.Since(start)

	response := PaginatedResponse{
		Nodes:       data.Nodes,
		Edges:       data.Edges,
		TotalNodes:  len(data.Nodes),
		TotalEdges:  len(data.Edges),
		Page:        1,
		PageSize:    len(data.Nodes),
		HasMore:     false,
		LoadTime:    loadTime,
		DiagramType: s.config.DiagramType,
	}

	s.writeJSON(w, response)
}

// getAllData returns the full Cytoscape dataset for the given diagram type, using an in-memory cache.
// If includeFullDepth is true for tracker-tree, it will compute with a very high recursion depth.
func (s *DiagramServer) getAllData(diagramType string, includeFullDepth bool) *spec.CytoscapeData {
	// Build cache key
	depthKey := "normal"
	if includeFullDepth {
		depthKey = "full"
	}
	cacheKey := fmt.Sprintf("%s:%s", diagramType, depthKey)

	if s.dataCache == nil {
		s.dataCache = make(map[string]*spec.CytoscapeData)
	}
	if cached, ok := s.dataCache[cacheKey]; ok && cached != nil {
		return cached
	}

	// Compute data
	var data *spec.CytoscapeData
	if diagramType == "tracker-tree" {
		// Choose depth
		maxDepth := s.config.MaxDepth
		if includeFullDepth {
			maxDepth = 1000
		}
		trackerTree := spec.NewTrackerTree(s.metadata, metadata.TrackerLimits{
			MaxNodesPerTree:    50000,
			MaxChildrenPerNode: 500,
			MaxArgsPerFunction: 100,
			MaxNestedArgsDepth: 100,
			MaxRecursionDepth:  maxDepth,
		})
		data = spec.DrawTrackerTreeCytoscapeWithMetadata(trackerTree.GetRoots(), s.metadata)
	} else {
		data = spec.DrawCallGraphCytoscape(s.metadata)
	}

	s.dataCache[cacheKey] = data
	return data
}

// handlePaginatedDiagram serves paginated diagram data
func (s *DiagramServer) handlePaginatedDiagram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if pageSize < 1 {
		pageSize = s.config.PageSize
	}
	if pageSize > 2000 {
		pageSize = 2000
	}

	depthStr := r.URL.Query().Get("depth")
	var depth int
	if depthStr == "" {
		depth = s.config.MaxDepth // Use default if not specified
	} else {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 0 {
			depth = 0 // Minimum depth is 0
		}
	}

	// Advanced search parameters
	packageFilter := r.URL.Query().Get("package")
	functionFilter := r.URL.Query().Get("function")
	fileFilter := r.URL.Query().Get("file")
	receiverFilter := r.URL.Query().Get("receiver")
	signatureFilter := r.URL.Query().Get("signature")
	genericFilter := r.URL.Query().Get("generic")
	scopeFilter := r.URL.Query().Get("scope")

	// Support multiple packages (comma-separated)
	packages := strings.Split(packageFilter, ",")
	if len(packages) == 1 && packages[0] == "" {
		packages = []string{}
	}

	// Support multiple values for other filters (comma-separated)
	functions := strings.Split(functionFilter, ",")
	if len(functions) == 1 && functions[0] == "" {
		functions = []string{}
	}

	files := strings.Split(fileFilter, ",")
	if len(files) == 1 && files[0] == "" {
		files = []string{}
	}

	receivers := strings.Split(receiverFilter, ",")
	if len(receivers) == 1 && receivers[0] == "" {
		receivers = []string{}
	}

	signatures := strings.Split(signatureFilter, ",")
	if len(signatures) == 1 && signatures[0] == "" {
		signatures = []string{}
	}

	generics := strings.Split(genericFilter, ",")
	if len(generics) == 1 && generics[0] == "" {
		generics = []string{}
	}

	// Generate paginated data
	data := s.generatePaginatedData(page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)

	loadTime := time.Since(start)

	response := PaginatedResponse{
		Nodes:       data.Nodes,
		Edges:       data.Edges,
		TotalNodes:  data.TotalNodes,
		TotalEdges:  data.TotalEdges,
		Page:        page,
		PageSize:    pageSize,
		HasMore:     data.HasMore,
		LoadTime:    loadTime,
		DiagramType: s.config.DiagramType,
	}

	s.writeJSON(w, response)
}

// PackageNode represents a package in the hierarchy
type PackageNode struct {
	Name     string        `json:"name"`
	FullPath string        `json:"full_path"`
	Count    int           `json:"count"`
	Children []PackageNode `json:"children,omitempty"`
}

// PackageHierarchyResponse represents package hierarchy
type PackageHierarchyResponse struct {
	RootPackages []PackageNode `json:"root_packages"`
	TotalCount   int           `json:"total_count"`
	DiagramType  string        `json:"diagram_type"` // "call-graph" or "tracker-tree"
}

// handlePackageHierarchy serves the package hierarchy
func (s *DiagramServer) handlePackageHierarchy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Use metadata.Packages directly to get ALL packages (not just ones with nodes in diagram)
	packageSet := make(map[string]bool)
	packageCounts := make(map[string]int)

	// Get all packages from metadata
	for pkgName := range s.metadata.Packages {
		packageSet[pkgName] = true
		packageCounts[pkgName] = 0 // Initialize count
	}

	// Also count nodes per package from actual diagram data for accurate counts
	allData := s.getAllData(s.config.DiagramType, true)

	// Count nodes per package
	for _, node := range allData.Nodes {
		pkg := node.Data.Package
		if pkg != "" {
			packageSet[pkg] = true
			packageCounts[pkg]++
		}
	}

	// Build parent-child relationships
	packageMap := make(map[string]map[string]bool) // parent -> children
	for pkg := range packageSet {
		parts := strings.Split(pkg, "/")
		for i := 1; i < len(parts); i++ {
			parent := strings.Join(parts[:i], "/")
			child := strings.Join(parts[:i+1], "/")
			if packageMap[parent] == nil {
				packageMap[parent] = make(map[string]bool)
			}
			packageMap[parent][child] = true
		}
	}

	// Find root packages (packages that are not children of any other package in our set)
	rootPackages := make(map[string]bool)
	for pkg := range packageSet {
		isRoot := true
		parts := strings.Split(pkg, "/")
		// Check if this package could be a child of any other package
		for i := 1; i < len(parts); i++ {
			potentialParent := strings.Join(parts[:i], "/")
			if packageSet[potentialParent] {
				isRoot = false
				break
			}
		}
		if isRoot {
			rootPackages[pkg] = true
		}
	}

	// Build tree structure
	var buildTree func(path string, depth int) PackageNode
	buildTree = func(path string, depth int) PackageNode {
		parts := strings.Split(path, "/")
		name := parts[len(parts)-1]
		if name == "" {
			name = path
		}

		node := PackageNode{
			Name:     name,
			FullPath: path,
			Count:    packageCounts[path],
		}

		if depth == 0 {
			node.Name = path
		}

		// Add children (direct sub-packages only)
		if children, exists := packageMap[path]; exists && depth < 20 {
			childPaths := make([]string, 0, len(children))
			for childPath := range children {
				// Verify this is a direct child (one level deeper)
				childParts := strings.Split(childPath, "/")
				if len(childParts) == len(parts)+1 {
					childPaths = append(childPaths, childPath)
				}
			}

			// Sort child paths
			sort.Slice(childPaths, func(i, j int) bool {
				return childPaths[i] < childPaths[j]
			})

			// Build children recursively
			node.Children = make([]PackageNode, 0, len(childPaths))
			for _, childPath := range childPaths {
				childNode := buildTree(childPath, depth+1)
				node.Count += childNode.Count // Aggregate counts
				node.Children = append(node.Children, childNode)
			}
		}

		return node
	}

	// Convert root packages map to sorted slice
	rootPaths := make([]string, 0, len(rootPackages))
	for pkg := range rootPackages {
		rootPaths = append(rootPaths, pkg)
	}
	sort.Slice(rootPaths, func(i, j int) bool {
		return rootPaths[i] < rootPaths[j]
	})

	// Build root nodes
	rootNodes := make([]PackageNode, 0, len(rootPaths))
	for _, rootPath := range rootPaths {
		rootNode := buildTree(rootPath, 0)
		// Include package even if it has no nodes, as long as it or its children exist
		if rootNode.Count > 0 || len(rootNode.Children) > 0 || packageSet[rootPath] {
			rootNodes = append(rootNodes, rootNode)
		}
	}

	totalCount := 0
	for _, count := range packageCounts {
		totalCount += count
	}

	response := PackageHierarchyResponse{
		RootPackages: rootNodes,
		TotalCount:   totalCount,
		DiagramType:  s.config.DiagramType,
	}

	s.writeJSON(w, response)
}

// handlePackageBasedDiagram serves diagram data filtered by selected packages
func (s *DiagramServer) handlePackageBasedDiagram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	start := time.Now()

	// Parse query parameters
	selectedPackages := r.URL.Query().Get("packages")
	if selectedPackages == "" {
		s.writeError(w, "packages parameter is required", http.StatusBadRequest)
		return
	}

	packages := strings.Split(selectedPackages, ",")
	for i := range packages {
		packages[i] = strings.TrimSpace(packages[i])
	}

	// Include child packages (packages that start with selected package paths)
	expandedPackages := make(map[string]bool)
	for _, pkg := range packages {
		expandedPackages[pkg] = true
		// Also include packages that start with this package path
		for pkgName := range s.metadata.Packages {
			// Check if this package is a child of the selected package
			if strings.HasPrefix(pkgName, pkg+"/") || pkgName == pkg {
				expandedPackages[pkgName] = true
			}
		}
	}

	// Convert to slice
	finalPackages := make([]string, 0, len(expandedPackages))
	for pkg := range expandedPackages {
		finalPackages = append(finalPackages, pkg)
	}

	// Get depth and other filters
	depthStr := r.URL.Query().Get("depth")
	var depth int
	if depthStr == "" {
		depth = s.config.MaxDepth // Use default if not specified
	} else {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 0 {
			depth = 0 // Minimum depth is 0
		}
	}

	functionFilter := r.URL.Query().Get("function")
	fileFilter := r.URL.Query().Get("file")
	receiverFilter := r.URL.Query().Get("receiver")
	signatureFilter := r.URL.Query().Get("signature")
	genericFilter := r.URL.Query().Get("generic")
	scopeFilter := r.URL.Query().Get("scope")

	// Support multiple values for filters
	functions := []string{}
	if functionFilter != "" {
		functions = strings.Split(functionFilter, ",")
		for i := range functions {
			functions[i] = strings.TrimSpace(functions[i])
		}
	}

	files := []string{}
	if fileFilter != "" {
		files = strings.Split(fileFilter, ",")
		for i := range files {
			files[i] = strings.TrimSpace(files[i])
		}
	}

	receivers := []string{}
	if receiverFilter != "" {
		receivers = strings.Split(receiverFilter, ",")
		for i := range receivers {
			receivers[i] = strings.TrimSpace(receivers[i])
		}
	}

	signatures := []string{}
	if signatureFilter != "" {
		signatures = strings.Split(signatureFilter, ",")
		for i := range signatures {
			signatures[i] = strings.TrimSpace(signatures[i])
		}
	}

	generics := []string{}
	if genericFilter != "" {
		generics = strings.Split(genericFilter, ",")
		for i := range generics {
			generics[i] = strings.TrimSpace(generics[i])
		}
	}

	// Check if isolate mode is enabled (only show selected packages, no relations)
	isolate := r.URL.Query().Get("isolate") == "true"

	// Generate data for selected packages
	data := s.generatePackageBasedData(finalPackages, depth, functions, files, receivers, signatures, generics, scopeFilter, isolate)

	loadTime := time.Since(start)

	response := PaginatedResponse{
		Nodes:       data.Nodes,
		Edges:       data.Edges,
		TotalNodes:  data.TotalNodes,
		TotalEdges:  data.TotalEdges,
		Page:        1,
		PageSize:    len(data.Nodes),
		HasMore:     false,
		LoadTime:    loadTime,
		DiagramType: s.config.DiagramType,
	}

	s.writeJSON(w, response)
}

// generatePackageBasedData generates diagram data filtered by selected packages
// If isolate is true, only includes nodes and edges within the selected packages (no external relations)
func (s *DiagramServer) generatePackageBasedData(selectedPackages []string, depth int, functions, files, receivers, signatures, generics []string, scopeFilter string, isolate bool) *spec.PaginatedCytoscapeData {
	// Generate all data first based on diagram type (cached)
	allData := s.getAllData(s.config.DiagramType, true)
	if s.config.DiagramType == "tracker-tree" {
		// Order nodes in depth-first order from root (main) to leaves for tracker-tree
		allData.Nodes = spec.OrderTrackerTreeNodesDepthFirst(allData)
	}

	// Note: For tracker-tree, we show full depth and order nodes depth-first
	// Depth parameter is ignored for tracker-tree mode

	// Apply depth filtering for call-graph mode (tracker-tree ignores depth and shows full tree)
	var depthFilteredNodes []spec.CytoscapeNode
	var depthFilteredEdges []spec.CytoscapeEdge

	if s.config.DiagramType == "call-graph" && depth >= 0 {
		// Calculate depth for call-graph nodes using BFS from root nodes
		nodeDepths := s.calculateCallGraphDepth(allData)

		// Filter nodes by depth
		nodeIDSet := make(map[string]bool)
		for _, node := range allData.Nodes {
			nodeDepth, hasDepth := nodeDepths[node.Data.ID]
			// Include nodes at or below the specified depth
			if hasDepth && nodeDepth <= depth {
				depthFilteredNodes = append(depthFilteredNodes, node)
				nodeIDSet[node.Data.ID] = true
			}
		}

		// Filter edges to only include those connecting filtered nodes
		for _, edge := range allData.Edges {
			if nodeIDSet[edge.Data.Source] && nodeIDSet[edge.Data.Target] {
				depthFilteredEdges = append(depthFilteredEdges, edge)
			}
		}
	} else {
		// For tracker-tree (which ignores depth), use all nodes
		depthFilteredNodes = allData.Nodes
		depthFilteredEdges = allData.Edges
	}

	// Filter nodes by selected packages (on depth-filtered nodes)
	var filteredNodes []spec.CytoscapeNode
	for _, node := range depthFilteredNodes {
		pkg := node.Data.Package
		matchPackage := false

		// Check if node's package matches any selected package or is a child
		for _, selectedPkg := range selectedPackages {
			if pkg == selectedPkg || strings.HasPrefix(pkg, selectedPkg+"/") {
				matchPackage = true
				break
			}
		}

		if !matchPackage {
			continue
		}

		// Apply other filters (same logic as generatePaginatedDataInternal)
		includeNode := true

		// Function filter (multi-value, word boundary matching)
		if len(functions) > 0 {
			functionMatch := false
			for _, function := range functions {
				if matchesFunctionName(node.Data.Label, function) {
					functionMatch = true
					break
				}
			}
			if !functionMatch {
				includeNode = false
			}
		}

		// File filter (multi-value, check both position field and call paths)
		// Node matches if ANY file filter matches EITHER Position OR any CallPath
		if len(files) > 0 {
			fileMatch := false
			// Check Position field
			if node.Data.Position != "" {
				for _, file := range files {
					if strings.Contains(strings.ToLower(node.Data.Position), strings.ToLower(strings.TrimSpace(file))) {
						fileMatch = true
						break
					}
				}
			}
			// Check CallPaths if Position didn't match
			if !fileMatch && len(node.Data.CallPaths) > 0 {
				for _, file := range files {
					for _, callPath := range node.Data.CallPaths {
						if strings.Contains(strings.ToLower(callPath.Position), strings.ToLower(strings.TrimSpace(file))) {
							fileMatch = true
							break
						}
					}
					if fileMatch {
						break
					}
				}
			}
			// Exclude node only if file filter is specified but no match found
			// (if node has no Position and no CallPaths, it's excluded when filter is specified)
			if !fileMatch {
				includeNode = false
			}
		}

		// Receiver filter
		if len(receivers) > 0 && node.Data.ReceiverType != "" {
			receiverMatch := false
			for _, receiver := range receivers {
				if strings.Contains(strings.ToLower(node.Data.ReceiverType), strings.ToLower(strings.TrimSpace(receiver))) {
					receiverMatch = true
					break
				}
			}
			if !receiverMatch {
				includeNode = false
			}
		}

		// Signature filter
		if len(signatures) > 0 && node.Data.SignatureStr != "" {
			signatureMatch := false
			for _, signature := range signatures {
				if strings.Contains(strings.ToLower(node.Data.SignatureStr), strings.ToLower(strings.TrimSpace(signature))) {
					signatureMatch = true
					break
				}
			}
			if !signatureMatch {
				includeNode = false
			}
		}

		// Generic filter
		if len(generics) > 0 && node.Data.Generics != nil {
			genericMatch := false
			for _, genericFilter := range generics {
				for _, generic := range node.Data.Generics {
					if strings.Contains(strings.ToLower(generic), strings.ToLower(strings.TrimSpace(genericFilter))) {
						genericMatch = true
						break
					}
				}
				if genericMatch {
					break
				}
			}
			if !genericMatch {
				includeNode = false
			}
		}

		// Scope filter (exported, unexported, all)
		if scopeFilter != "" && scopeFilter != "all" {
			nodeScope := strings.ToLower(strings.TrimSpace(node.Data.Scope))
			scopeFilterLower := strings.ToLower(strings.TrimSpace(scopeFilter))

			switch scopeFilterLower {
			case "exported":
				// Only include if scope is exactly "exported"
				if nodeScope != "exported" {
					includeNode = false
				}
			case "unexported":
				// Only include if scope is exactly "unexported"
				if nodeScope != "unexported" {
					includeNode = false
				}
			}
		}

		if includeNode {
			filteredNodes = append(filteredNodes, node)
		}
	}

	// Filter edges to only include those between filtered nodes
	nodeIDs := make(map[string]bool)
	for _, node := range filteredNodes {
		nodeIDs[node.Data.ID] = true
	}

	var filteredEdges []spec.CytoscapeEdge

	if isolate {
		// Isolate mode: only include edges between nodes in the selected packages
		// Use depthFilteredEdges instead of allData.Edges to respect depth filtering
		for _, edge := range depthFilteredEdges {
			sourceInSet := nodeIDs[edge.Data.Source]
			targetInSet := nodeIDs[edge.Data.Target]

			// Only include edge if both nodes are in filtered set
			if sourceInSet && targetInSet {
				filteredEdges = append(filteredEdges, edge)
			}
		}
	} else {
		// Normal mode: include nodes connected to filtered nodes (to show inter-package relationships)
		connectedNodeIDs := make(map[string]bool)

		// Use depthFilteredEdges instead of allData.Edges to respect depth filtering
		for _, edge := range depthFilteredEdges {
			sourceInSet := nodeIDs[edge.Data.Source]
			targetInSet := nodeIDs[edge.Data.Target]

			// Include edge if both nodes are in filtered set
			if sourceInSet && targetInSet {
				filteredEdges = append(filteredEdges, edge)
			} else if sourceInSet || targetInSet {
				// Include connected node to show relationships
				connectedID := edge.Data.Source
				if sourceInSet {
					connectedID = edge.Data.Target
				}

				// Find the connected node
				for _, node := range allData.Nodes {
					if node.Data.ID == connectedID {
						// Add connected node and edge if not already added
						if !nodeIDs[node.Data.ID] && !connectedNodeIDs[node.Data.ID] {
							connectedNodeIDs[node.Data.ID] = true
							filteredNodes = append(filteredNodes, node)
							nodeIDs[node.Data.ID] = true
						}
						filteredEdges = append(filteredEdges, edge)
						break
					}
				}
			}
		}
	}

	return &spec.PaginatedCytoscapeData{
		Nodes:      filteredNodes,
		Edges:      filteredEdges,
		TotalNodes: len(filteredNodes),
		TotalEdges: len(filteredEdges),
		Page:       1,
		PageSize:   len(filteredNodes),
		HasMore:    false,
	}
}

// handleStats serves diagram statistics
func (s *DiagramServer) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	stats := map[string]interface{}{
		"total_nodes":     len(s.metadata.Packages),
		"total_edges":     len(s.metadata.CallGraph),
		"total_functions": len(s.metadata.Packages),
		"last_load":       s.lastLoad.Format(time.RFC3339),
		"cache_timeout":   s.config.CacheTimeout.String(),
		"page_size":       s.config.PageSize,
		"max_depth":       s.config.MaxDepth,
		"input_dir":       s.config.InputDir,
	}

	s.writeJSON(w, stats)
}

// handleRefresh refreshes the metadata
func (s *DiagramServer) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("🔄 Refreshing metadata...")

	if err := s.LoadMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to refresh metadata: %v", err), http.StatusInternalServerError)
		return
	}

	// Clear cache
	s.cache = make(map[string]*spec.PaginatedCytoscapeData)
	s.dataCache = make(map[string]*spec.CytoscapeData)

	response := map[string]interface{}{
		"message":   "Metadata refreshed successfully",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	s.writeJSON(w, response)
}

// handleExport serves diagram export in various formats
func (s *DiagramServer) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	format := r.URL.Query().Get("format")
	if format == "" {
		format = "svg"
	}

	// Validate format
	validFormats := map[string]string{
		"svg":  "image/svg+xml",
		"png":  "image/png",
		"jpg":  "image/jpeg",
		"pdf":  "application/pdf",
		"json": "application/json",
	}

	contentType, exists := validFormats[format]
	if !exists {
		s.writeError(w, "Invalid format. Supported formats: svg, png, jpg, pdf, json", http.StatusBadRequest)
		return
	}

	// Get the same parameters as paginated diagram
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	pageSize, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if pageSize < 1 {
		pageSize = s.config.PageSize
	}
	if pageSize > 2000 {
		pageSize = 2000
	}

	depthStr := r.URL.Query().Get("depth")
	var depth int
	if depthStr == "" {
		depth = s.config.MaxDepth // Use default if not specified
	} else {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 0 {
			depth = 0 // Minimum depth is 0
		}
	}

	// Advanced search parameters
	packageFilter := r.URL.Query().Get("package")
	functionFilter := r.URL.Query().Get("function")
	fileFilter := r.URL.Query().Get("file")
	receiverFilter := r.URL.Query().Get("receiver")
	signatureFilter := r.URL.Query().Get("signature")
	genericFilter := r.URL.Query().Get("generic")
	scopeFilter := r.URL.Query().Get("scope")

	// Support multiple packages (comma-separated)
	packages := strings.Split(packageFilter, ",")
	if len(packages) == 1 && packages[0] == "" {
		packages = []string{}
	}

	// Support multiple values for other filters (comma-separated)
	functions := strings.Split(functionFilter, ",")
	if len(functions) == 1 && functions[0] == "" {
		functions = []string{}
	}

	files := strings.Split(fileFilter, ",")
	if len(files) == 1 && files[0] == "" {
		files = []string{}
	}

	receivers := strings.Split(receiverFilter, ",")
	if len(receivers) == 1 && receivers[0] == "" {
		receivers = []string{}
	}

	signatures := strings.Split(signatureFilter, ",")
	if len(signatures) == 1 && signatures[0] == "" {
		signatures = []string{}
	}

	generics := strings.Split(genericFilter, ",")
	if len(generics) == 1 && generics[0] == "" {
		generics = []string{}
	}

	// Generate data
	data := s.generatePaginatedData(page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)

	// Set appropriate headers
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"diagram.%s\"", format))

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	// Generate the appropriate format
	// Note: Most formats (SVG, PNG, JPG, PDF) are now handled client-side using Cytoscape.js extensions
	// Only JSON export is handled server-side for programmatic access
	switch format {
	case "json":
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			s.writeError(w, "Failed to generate JSON export", http.StatusInternalServerError)
			return
		}
		_, err = w.Write(jsonData)
		if err != nil {
			s.writeError(w, "Failed to write JSON export", http.StatusInternalServerError)
			return
		}
		return

	default:
		// For other formats, return a message directing users to use the client-side export
		message := fmt.Sprintf("Format '%s' is now handled client-side using Cytoscape.js extensions. Please use the export dropdown in the UI.", format)
		s.writeError(w, message, http.StatusBadRequest)
		return
	}
}

// handleHealth serves health check
func (s *DiagramServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	health := map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().Format(time.RFC3339),
		"uptime":    time.Since(s.lastLoad).String(),
	}

	s.writeJSON(w, health)
}

// calculateCallGraphDepth performs BFS from root nodes to calculate depth for each node
func (s *DiagramServer) calculateCallGraphDepth(data *spec.CytoscapeData) map[string]int {
	depths := make(map[string]int)

	// Build adjacency list from edges
	adjacency := make(map[string][]string)
	inDegree := make(map[string]int)

	// Initialize all nodes
	for _, node := range data.Nodes {
		inDegree[node.Data.ID] = 0
		adjacency[node.Data.ID] = []string{}
	}

	// Build the graph
	for _, edge := range data.Edges {
		adjacency[edge.Data.Source] = append(adjacency[edge.Data.Source], edge.Data.Target)
		inDegree[edge.Data.Target]++
	}

	// Find root nodes (nodes with no incoming edges, or nodes labeled "main")
	var roots []string
	for _, node := range data.Nodes {
		if inDegree[node.Data.ID] == 0 || node.Data.Label == "main" || node.Data.FunctionName == "main" {
			roots = append(roots, node.Data.ID)
		}
	}

	// If no roots found, use all nodes with zero in-degree
	if len(roots) == 0 {
		for nodeID, degree := range inDegree {
			if degree == 0 {
				roots = append(roots, nodeID)
			}
		}
	}

	// BFS from all root nodes to assign depths
	queue := make([]struct {
		nodeID string
		depth  int
	}, 0)

	for _, root := range roots {
		queue = append(queue, struct {
			nodeID string
			depth  int
		}{root, 0})
		depths[root] = 0
	}

	visited := make(map[string]bool)
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		if visited[current.nodeID] {
			continue
		}
		visited[current.nodeID] = true

		// Visit all neighbors
		for _, neighbor := range adjacency[current.nodeID] {
			newDepth := current.depth + 1

			// Update depth if this is the first time visiting or if we found a shorter path
			if existingDepth, exists := depths[neighbor]; !exists || newDepth < existingDepth {
				depths[neighbor] = newDepth
				queue = append(queue, struct {
					nodeID string
					depth  int
				}{neighbor, newDepth})
			}
		}
	}

	return depths
}

// generatePaginatedData generates paginated diagram data with depth filtering and advanced search
func (s *DiagramServer) generatePaginatedData(page, pageSize, depth int, packages, functions, files, receivers, signatures, generics []string, scopeFilter string) *spec.PaginatedCytoscapeData {
	// Add timeout for large depth requests
	timeout := time.After(30 * time.Second)
	done := make(chan *spec.PaginatedCytoscapeData, 1)

	go func() {
		result := s.generatePaginatedDataInternal(page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)
		done <- result
	}()

	select {
	case result := <-done:
		return result
	case <-timeout:
		// Return a limited result if timeout occurs
		return &spec.PaginatedCytoscapeData{
			Nodes:      []spec.CytoscapeNode{},
			Edges:      []spec.CytoscapeEdge{},
			TotalNodes: 0,
			TotalEdges: 0,
			Page:       page,
			PageSize:   pageSize,
			HasMore:    false,
		}
	}
}

// generatePaginatedDataInternal is the internal implementation without timeout
func (s *DiagramServer) generatePaginatedDataInternal(page, pageSize, depth int, packages, functions, files, receivers, signatures, generics []string, scopeFilter string) *spec.PaginatedCytoscapeData {
	// Check cache first
	cacheKey := fmt.Sprintf("%s-%d-%d-%d-%v-%v-%v-%v-%v-%v-%s", s.config.DiagramType, page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)
	if cached, exists := s.cache[cacheKey]; exists {
		return cached
	}

	// Generate all data first based on diagram type (cached)
	allData := s.getAllData(s.config.DiagramType, true)
	if s.config.DiagramType == "tracker-tree" {
		// Order nodes in depth-first order from root (main) to leaves for tracker-tree
		allData.Nodes = spec.OrderTrackerTreeNodesDepthFirst(allData)
	}

	// Note: For tracker-tree, we show full depth and order nodes depth-first for pagination
	// Depth parameter is ignored for tracker-tree mode

	allNodes := make(map[string]*spec.CytoscapeNode)
	for _, node := range allData.Nodes {
		allNodes[node.Data.ID] = &node
	}

	// Apply depth filtering for call-graph mode (tracker-tree ignores depth and shows full tree)
	var depthFilteredNodes []spec.CytoscapeNode
	var depthFilteredEdges []spec.CytoscapeEdge

	if s.config.DiagramType == "call-graph" && depth >= 0 {
		// Calculate depth for call-graph nodes using BFS from root nodes
		nodeDepths := s.calculateCallGraphDepth(allData)

		// Debug: log calculated depths
		if s.config.Verbose {
			log.Printf("Calculated depths for %d nodes, filtering to depth %d", len(nodeDepths), depth)
			for nodeID, d := range nodeDepths {
				for _, node := range allData.Nodes {
					if node.Data.ID == nodeID {
						log.Printf("  Node %s (%s) -> depth %d", nodeID, node.Data.Label, d)
						break
					}
				}
			}
		}

		// Filter nodes by depth
		nodeIDSet := make(map[string]bool)
		includedCount := 0
		excludedCount := 0
		for _, node := range allData.Nodes {
			nodeDepth, hasDepth := nodeDepths[node.Data.ID]
			// Include nodes at or below the specified depth
			if hasDepth && nodeDepth <= depth {
				depthFilteredNodes = append(depthFilteredNodes, node)
				nodeIDSet[node.Data.ID] = true
				includedCount++
			} else {
				excludedCount++
			}
		}

		if s.config.Verbose {
			log.Printf("Depth filtering: included %d nodes, excluded %d nodes (depth limit: %d)", includedCount, excludedCount, depth)
		}

		// Filter edges to only include those connecting filtered nodes
		for _, edge := range allData.Edges {
			if nodeIDSet[edge.Data.Source] && nodeIDSet[edge.Data.Target] {
				depthFilteredEdges = append(depthFilteredEdges, edge)
			}
		}
	} else {
		// For tracker-tree (which ignores depth), use all nodes
		depthFilteredNodes = allData.Nodes
		depthFilteredEdges = allData.Edges
	}

	// Apply package and function filters
	var filteredNodes []spec.CytoscapeNode
	var filteredEdges []spec.CytoscapeEdge

	// Apply advanced filters
	for _, node := range depthFilteredNodes {
		includeNode := true

		// Multiple package filter (OR logic - matches any of the packages)
		if len(packages) > 0 {
			packageMatch := false
			for _, pkg := range packages {
				if strings.Contains(node.Data.Package, strings.TrimSpace(pkg)) {
					packageMatch = true
					break
				}
			}
			if !packageMatch {
				includeNode = false
			}
		}

		// Function filter (multi-value, word boundary matching)
		if len(functions) > 0 {
			functionMatch := false
			for _, function := range functions {
				if matchesFunctionName(node.Data.Label, function) {
					functionMatch = true
					break
				}
			}
			if !functionMatch {
				includeNode = false
			}
		}

		// File filter (multi-value, check both position field and call paths)
		// Node matches if ANY file filter matches EITHER Position OR any CallPath
		if len(files) > 0 {
			fileMatch := false
			// Check Position field
			if node.Data.Position != "" {
				for _, file := range files {
					if strings.Contains(strings.ToLower(node.Data.Position), strings.ToLower(strings.TrimSpace(file))) {
						fileMatch = true
						break
					}
				}
			}
			// Check CallPaths if Position didn't match
			if !fileMatch && len(node.Data.CallPaths) > 0 {
				for _, file := range files {
					for _, callPath := range node.Data.CallPaths {
						if strings.Contains(strings.ToLower(callPath.Position), strings.ToLower(strings.TrimSpace(file))) {
							fileMatch = true
							break
						}
					}
					if fileMatch {
						break
					}
				}
			}
			// Exclude node only if file filter is specified but no match found
			// (if node has no Position and no CallPaths, it's excluded when filter is specified)
			if !fileMatch {
				includeNode = false
			}
		}

		// Receiver filter (multi-value)
		if len(receivers) > 0 && node.Data.ReceiverType != "" {
			receiverMatch := false
			for _, receiver := range receivers {
				if strings.Contains(strings.ToLower(node.Data.ReceiverType), strings.ToLower(strings.TrimSpace(receiver))) {
					receiverMatch = true
					break
				}
			}
			if !receiverMatch {
				includeNode = false
			}
		}

		// Signature filter (multi-value)
		if len(signatures) > 0 && node.Data.SignatureStr != "" {
			signatureMatch := false
			for _, signature := range signatures {
				if strings.Contains(strings.ToLower(node.Data.SignatureStr), strings.ToLower(strings.TrimSpace(signature))) {
					signatureMatch = true
					break
				}
			}
			if !signatureMatch {
				includeNode = false
			}
		}

		// Generic filter (multi-value, check generics field)
		if len(generics) > 0 && node.Data.Generics != nil {
			genericMatch := false
			for _, genericFilter := range generics {
				for _, generic := range node.Data.Generics {
					if strings.Contains(strings.ToLower(generic), strings.ToLower(strings.TrimSpace(genericFilter))) {
						genericMatch = true
						break
					}
				}
				if genericMatch {
					break
				}
			}
			if !genericMatch {
				includeNode = false
			}
		}

		// Scope filter (exported, unexported, all)
		if scopeFilter != "" && scopeFilter != "all" {
			nodeScope := strings.ToLower(strings.TrimSpace(node.Data.Scope))
			scopeFilterLower := strings.ToLower(strings.TrimSpace(scopeFilter))

			switch scopeFilterLower {
			case "exported":
				// Only include if scope is exactly "exported"
				if nodeScope != "exported" {
					includeNode = false
				}
			case "unexported":
				// Only include if scope is exactly "unexported"
				if nodeScope != "unexported" {
					includeNode = false
				}
			}
		}

		if includeNode {
			filteredNodes = append(filteredNodes, node)
		}
	}

	// Filter edges to only include those between filtered nodes
	nodeIDs := make(map[string]*spec.CytoscapeNode)
	for _, node := range filteredNodes {
		nodeIDs[node.Data.ID] = &node
	}

	for _, edge := range depthFilteredEdges {
		if nodeIDs[edge.Data.Source] != nil && nodeIDs[edge.Data.Target] != nil {
			filteredEdges = append(filteredEdges, edge)
		}
	}

	// Apply pagination. For tracker-tree, order nodes branch-by-branch depth-first, then paginate.
	var paginatedNodes []spec.CytoscapeNode
	var start, end int
	var orderedNodes []spec.CytoscapeNode // Used for tracker-tree ordering
	if s.config.DiagramType == "tracker-tree" {
		// Build filtered data for branch-ordered traversal
		filteredData := &spec.CytoscapeData{Nodes: filteredNodes, Edges: filteredEdges}

		// Get nodes in branch-first order (complete one branch before moving to next)
		orderedNodes = spec.TraverseTrackerTreeBranchOrder(filteredData)

		// Paginate the ordered nodes
		start = (page - 1) * pageSize
		end = start + pageSize
		if start < len(orderedNodes) {
			if end > len(orderedNodes) {
				end = len(orderedNodes)
			}
			paginatedNodes = orderedNodes[start:end]
		}
	} else {
		// Default (call-graph) pagination by slice
		orderedNodes = filteredNodes // For consistency in later code
		start = (page - 1) * pageSize
		end = start + pageSize
		if start < len(filteredNodes) {
			if end > len(filteredNodes) {
				end = len(filteredNodes)
			}
			paginatedNodes = filteredNodes[start:end]
		}
	}

	// Create a map of paginated node IDs for quick lookup
	paginatedNodeIDs := make(map[string]*spec.CytoscapeNode)
	for _, node := range paginatedNodes {
		paginatedNodeIDs[node.Data.ID] = &node
	}

	// Include edges that connect paginated nodes, but also include edges
	// that connect to nodes from previous pages (to maintain graph connectivity)
	var paginatedEdges []spec.CytoscapeEdge

	// First pass: collect all edges between paginated nodes
	for _, edge := range filteredEdges {
		if paginatedNodeIDs[edge.Data.Source] != nil && paginatedNodeIDs[edge.Data.Target] != nil {
			paginatedEdges = append(paginatedEdges, edge)
		}
	}

	for _, node := range paginatedNodes {
		if node.Data.Parent != "" && paginatedNodeIDs[node.Data.Parent] == nil {
			parentNode := allNodes[node.Data.Parent]
			if parentNode != nil {
				parentNode.Data.IsParentFunction = "true"
				paginatedNodeIDs[node.Data.Parent] = parentNode
				paginatedNodes = append(paginatedNodes, *parentNode)
			} else {
				fmt.Printf("Parent node not found: %s\n", node.Data.Parent)
			}
		}
	}

	result := &spec.PaginatedCytoscapeData{
		Nodes:      paginatedNodes,
		Edges:      paginatedEdges,
		TotalNodes: len(filteredNodes),
		TotalEdges: len(filteredEdges),
		Page:       page,
		PageSize:   pageSize,
		HasMore:    len(paginatedNodes) < len(filteredNodes),
	}

	// Cache result
	s.cache[cacheKey] = result

	return result
}

// writeJSON writes JSON response
func (s *DiagramServer) writeJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Failed to encode JSON: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
	}
}

// writeResponse writes a response with content type
func (s *DiagramServer) writeResponse(w http.ResponseWriter, data string, contentType string) {
	w.Header().Set("Content-Type", contentType)

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	_, err := w.Write([]byte(data))
	if err != nil {
		s.writeError(w, "Failed to write response", http.StatusInternalServerError)
		return
	}
}

// writeError writes an error response
func (s *DiagramServer) writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	errorResp := ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
		Code:    code,
	}

	err := json.NewEncoder(w).Encode(errorResp)
	if err != nil {
		s.writeError(w, "Failed to write error response", http.StatusInternalServerError)
		return
	}
}
