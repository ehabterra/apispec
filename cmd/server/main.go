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
	"strconv"
	"strings"
	"time"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

//go:embed server_ui.html
var serverUITemplate embed.FS

// ServerConfig holds configuration for the diagram server
type ServerConfig struct {
	Port         int
	Host         string
	InputDir     string
	PageSize     int
	MaxDepth     int
	EnableCORS   bool
	CacheTimeout time.Duration
	StaticDir    string
	Verbose      bool
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
	log.Printf("🚀 Diagram server starting on http://%s", addr)
	log.Printf("📊 Serving paginated diagrams for: %s", config.InputDir)
	log.Printf("⚙️  Page size: %d, Max depth: %d", config.PageSize, config.MaxDepth)

	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}

func parseFlags() *ServerConfig {
	config := &ServerConfig{}

	flag.IntVar(&config.Port, "port", 8080, "Server port")
	flag.StringVar(&config.Host, "host", "localhost", "Server host")
	flag.StringVar(&config.InputDir, "dir", ".", "Input directory containing Go source files")
	flag.IntVar(&config.PageSize, "page-size", 100, "Default page size for pagination")
	flag.IntVar(&config.MaxDepth, "max-depth", 3, "Maximum call graph depth")
	flag.BoolVar(&config.EnableCORS, "cors", true, "Enable CORS headers")
	flag.DurationVar(&config.CacheTimeout, "cache-timeout", 5*time.Minute, "Cache timeout for metadata")
	flag.StringVar(&config.StaticDir, "static", "", "Directory to serve static files from")
	flag.BoolVar(&config.Verbose, "verbose", false, "Enable verbose logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "APISpec Diagram Server - Serves paginated call graph diagrams\n\n")
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
		InputDir:                     s.config.InputDir,
		MaxNodesPerTree:              50000,
		MaxChildrenPerNode:           500,
		MaxArgsPerFunction:           100,
		MaxNestedArgsDepth:           100,
		MaxRecursionDepth:            s.config.MaxDepth,
		SkipCGOPackages:              true,
		AnalyzeFrameworkDependencies: true,
		AutoIncludeFrameworkPackages: true,
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
	log.Printf("📊 Total packages: %d", len(s.metadata.Packages))
	log.Printf("📊 Total call graph edges: %d", len(s.metadata.CallGraph))

	return nil
}

// SetupRoutes sets up HTTP routes
func (s *DiagramServer) SetupRoutes() {
	// API routes
	http.HandleFunc("/api/diagram", s.handleDiagram)
	http.HandleFunc("/api/diagram/page", s.handlePaginatedDiagram)
	http.HandleFunc("/api/diagram/stats", s.handleStats)
	http.HandleFunc("/api/diagram/refresh", s.handleRefresh)

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
	if pageSize > 1000 {
		pageSize = 1000
	}

	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if depth < 1 {
		depth = s.config.MaxDepth
	}

	packageFilter := r.URL.Query().Get("package")
	functionFilter := r.URL.Query().Get("function")

	// Generate paginated data
	data := s.generatePaginatedData(page, pageSize, depth, packageFilter, functionFilter)

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

// generatePaginatedData generates paginated diagram data
func (s *DiagramServer) generatePaginatedData(page, pageSize, depth int, packageFilter, functionFilter string) *spec.PaginatedCytoscapeData {
	// Check cache first
	cacheKey := fmt.Sprintf("%d-%d-%d-%s-%s", page, pageSize, depth, packageFilter, functionFilter)
	if cached, exists := s.cache[cacheKey]; exists {
		return cached
	}

	// Generate all data first
	allData := spec.DrawCallGraphCytoscape(s.metadata)

	// Apply filters
	var filteredNodes []spec.CytoscapeNode
	var filteredEdges []spec.CytoscapeEdge

	// Apply package and function filters
	for _, node := range allData.Nodes {
		includeNode := true

		// Package filter
		if packageFilter != "" && !strings.Contains(node.Data.Package, packageFilter) {
			includeNode = false
		}

		// Function filter
		if functionFilter != "" && !strings.Contains(node.Data.Label, functionFilter) {
			includeNode = false
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

	for _, edge := range allData.Edges {
		if nodeIDs[edge.Data.Source] && nodeIDs[edge.Data.Target] {
			filteredEdges = append(filteredEdges, edge)
		}
	}

	// Apply pagination
	start := (page - 1) * pageSize
	end := start + pageSize

	var paginatedNodes []spec.CytoscapeNode
	if start < len(filteredNodes) {
		if end > len(filteredNodes) {
			end = len(filteredNodes)
		}
		paginatedNodes = filteredNodes[start:end]
	}

	// Only include edges between paginated nodes
	paginatedNodeIDs := make(map[string]bool)
	for _, node := range paginatedNodes {
		paginatedNodeIDs[node.Data.ID] = true
	}

	var paginatedEdges []spec.CytoscapeEdge
	for _, edge := range filteredEdges {
		if paginatedNodeIDs[edge.Data.Source] && paginatedNodeIDs[edge.Data.Target] {
			paginatedEdges = append(paginatedEdges, edge)
		}
	}

	result := &spec.PaginatedCytoscapeData{
		Nodes:      paginatedNodes,
		Edges:      paginatedEdges,
		TotalNodes: len(filteredNodes),
		TotalEdges: len(filteredEdges),
		Page:       page,
		PageSize:   pageSize,
		HasMore:    end < len(filteredNodes),
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

	w.Write([]byte(data))
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

	json.NewEncoder(w).Encode(errorResp)
}
