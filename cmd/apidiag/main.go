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
}

// DiagramServer handles HTTP requests for paginated diagram data
type DiagramServer struct {
	config   *ServerConfig
	metadata *metadata.Metadata
	cache    map[string]*spec.PaginatedCytoscapeData
	lastLoad time.Time
}

// PaginatedResponse represents a paginated response
type PaginatedResponse struct {
	Nodes      []spec.CytoscapeNode `json:"nodes"`
	Edges      []spec.CytoscapeEdge `json:"edges"`
	TotalNodes int                  `json:"total_nodes"`
	TotalEdges int                  `json:"total_edges"`
	Page       int                  `json:"page"`
	PageSize   int                  `json:"page_size"`
	HasMore    bool                 `json:"has_more"`
	LoadTime   time.Duration        `json:"load_time_ms"`
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
		log.Printf("📊 Serving paginated diagrams for: %s", config.InputDir)
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

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "APISpec API Diagram Server - Serves paginated call graph diagrams\n\n")
		fmt.Fprintf(os.Stderr, "Usage: %s [flags]\n\nFlags:\n", os.Args[0])
		flag.PrintDefaults()
		fmt.Fprintf(os.Stderr, "\nExamples:\n")
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --port 8080\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "  %s --dir ./myproject --page-size 50 --max-depth 2\n", os.Args[0])
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

	return config
}

// NewDiagramServer creates a new diagram server
func NewDiagramServer(config *ServerConfig) *DiagramServer {
	return &DiagramServer{
		config: config,
		cache:  make(map[string]*spec.PaginatedCytoscapeData),
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

	// Generate complete diagram data
	data := spec.DrawCallGraphCytoscape(s.metadata)

	loadTime := time.Since(start)

	response := PaginatedResponse{
		Nodes:      data.Nodes,
		Edges:      data.Edges,
		TotalNodes: len(data.Nodes),
		TotalEdges: len(data.Edges),
		Page:       1,
		PageSize:   len(data.Nodes),
		HasMore:    false,
		LoadTime:   loadTime,
	}

	s.writeJSON(w, response)
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

	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if depth < 1 {
		depth = s.config.MaxDepth
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
		Nodes:      data.Nodes,
		Edges:      data.Edges,
		TotalNodes: data.TotalNodes,
		TotalEdges: data.TotalEdges,
		Page:       page,
		PageSize:   pageSize,
		HasMore:    data.HasMore,
		LoadTime:   loadTime,
	}

	s.writeJSON(w, response)
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

	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if depth < 1 {
		depth = s.config.MaxDepth
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

// generatePaginatedData generates paginated diagram data with depth filtering and advanced search
func (s *DiagramServer) generatePaginatedData(page, pageSize, depth int, packages, functions, files, receivers, signatures, generics []string, scopeFilter string) *spec.PaginatedCytoscapeData {
	// Check cache first
	cacheKey := fmt.Sprintf("%d-%d-%d-%v-%v-%v-%v-%v-%v-%s", page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)
	if cached, exists := s.cache[cacheKey]; exists {
		return cached
	}

	// Generate all data first
	allData := spec.DrawCallGraphCytoscape(s.metadata)

	// Apply depth filtering first
	var depthFilteredNodes []spec.CytoscapeNode
	var depthFilteredEdges []spec.CytoscapeEdge

	allNodes := make(map[string]*spec.CytoscapeNode)

	for _, node := range allData.Nodes {
		allNodes[node.Data.ID] = &node
	}

	if depth > 0 && depth < 10 { // Only apply depth filtering if depth is reasonable
		// Get root functions (functions with no incoming edges)
		rootNodes := make(map[string]bool)
		hasIncoming := make(map[string]bool)

		for _, edge := range allData.Edges {
			hasIncoming[edge.Data.Target] = true
		}

		for _, node := range allData.Nodes {
			if !hasIncoming[node.Data.ID] {
				rootNodes[node.Data.ID] = true
			}
		}

		// BFS to find nodes within depth limit
		visited := make(map[string]bool)
		queue := make([]string, 0)
		nodeDepths := make(map[string]int)

		// Start with root nodes
		for rootID := range rootNodes {
			queue = append(queue, rootID)
			nodeDepths[rootID] = 0
			visited[rootID] = true
		}

		// BFS traversal
		for len(queue) > 0 {
			currentID := queue[0]
			queue = queue[1:]
			currentDepth := nodeDepths[currentID]

			if currentDepth >= depth {
				continue
			}

			// Find the node and add it
			if node, ok := allNodes[currentID]; ok {
				depthFilteredNodes = append(depthFilteredNodes, *node)
			}

			// Add connected nodes
			for _, edge := range allData.Edges {
				if edge.Data.Source == currentID && !visited[edge.Data.Target] {
					visited[edge.Data.Target] = true
					nodeDepths[edge.Data.Target] = currentDepth + 1
					queue = append(queue, edge.Data.Target)

					// Add the edge
					depthFilteredEdges = append(depthFilteredEdges, edge)
				}
			}
		}
	} else {
		// No depth filtering, use all data
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

		// Function filter (multi-value)
		if len(functions) > 0 {
			functionMatch := false
			for _, function := range functions {
				if strings.Contains(strings.ToLower(node.Data.Label), strings.ToLower(strings.TrimSpace(function))) {
					functionMatch = true
					break
				}
			}
			if !functionMatch {
				includeNode = false
			}
		}

		// File filter (multi-value, check position field)
		if len(files) > 0 && node.Data.Position != "" {
			fileMatch := false
			for _, file := range files {
				if strings.Contains(strings.ToLower(node.Data.Position), strings.ToLower(strings.TrimSpace(file))) {
					fileMatch = true
					break
				}
			}
			if !fileMatch {
				includeNode = false
			}
		}

		// File filter (multi-value, check call paths field)
		if len(files) > 0 && len(node.Data.CallPaths) > 0 {
			fileMatch := false
			for _, file := range files {
				for _, callPath := range node.Data.CallPaths {
					if strings.Contains(strings.ToLower(callPath.Position), strings.ToLower(strings.TrimSpace(file))) {
						fileMatch = true
						break
					}
				}
			}
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
			nodeScope := node.Data.Scope

			if scopeFilter == "exported" && nodeScope != "exported" {
				includeNode = false
			} else if scopeFilter == "unexported" && nodeScope != "unexported" {
				includeNode = false
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

	// Apply pagination with better edge handling
	start := (page - 1) * pageSize
	end := start + pageSize

	var paginatedNodes []spec.CytoscapeNode
	if start < len(filteredNodes) {
		if end > len(filteredNodes) {
			end = len(filteredNodes)
		}
		paginatedNodes = filteredNodes[start:end]
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

	// Second pass: include edges that connect paginated nodes to other filtered nodes
	// This helps maintain graph connectivity across pages
	for _, edge := range filteredEdges {
		sourceInPage := paginatedNodeIDs[edge.Data.Source]
		targetInPage := paginatedNodeIDs[edge.Data.Target]

		// Include edge if at least one node is in current page
		if sourceInPage != nil || targetInPage != nil {
			// Check if we already have this edge
			edgeExists := false
			for _, existingEdge := range paginatedEdges {
				if existingEdge.Data.Source == edge.Data.Source && existingEdge.Data.Target == edge.Data.Target {
					edgeExists = true
					break
				}
			}
			if !edgeExists {
				paginatedEdges = append(paginatedEdges, edge)

				// Track connected nodes that aren't in current page
				if sourceInPage == nil {
					node := nodeIDs[edge.Data.Source]
					paginatedNodeIDs[edge.Data.Source] = node
					paginatedNodes = append(paginatedNodes, *node)
				}
				if targetInPage == nil {
					node := nodeIDs[edge.Data.Target]
					paginatedNodeIDs[edge.Data.Target] = node
					paginatedNodes = append(paginatedNodes, *node)
				}
			}
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
