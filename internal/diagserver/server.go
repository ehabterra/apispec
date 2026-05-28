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

// Package diagserver hosts the call-graph / tracker-tree diagram HTTP server
// used by both the standalone apidiag binary and the apispecui binary.
package diagserver

import (
	"compress/gzip"
	"embed"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ehabterra/apispec/internal/engine"
	"github.com/ehabterra/apispec/internal/metadata"
	"github.com/ehabterra/apispec/internal/spec"
)

//go:embed server_ui.html
var serverUITemplate embed.FS

// Config holds the configuration for a diagram server instance.
type Config struct {
	Host                         string
	Port                         int
	InputDir                     string
	PageSize                     int
	MaxDepth                     int
	EnableCORS                   bool
	CacheTimeout                 time.Duration
	Verbose                      bool
	AnalyzeFrameworkDependencies bool
	AutoIncludeFrameworkPackages bool
	AutoExcludeTests             bool
	AutoExcludeMocks             bool
	DiagramType                  string // "call-graph" or "tracker-tree"
}

// RouteOptions controls how the server's routes are mounted on a mux.
type RouteOptions struct {
	// UIPath is the path at which the interactive HTML UI is served.
	// Defaults to "/" if empty.
	UIPath string
	// APIPrefix is the prefix for the JSON API. Defaults to "/api/diagram".
	// Routes registered: <APIPrefix>, <APIPrefix>/page, <APIPrefix>/packages,
	// <APIPrefix>/by-packages, <APIPrefix>/stats, <APIPrefix>/refresh,
	// <APIPrefix>/export.
	APIPrefix string
	// HealthPath is the health-check endpoint. Defaults to "/health".
	// Set to empty string to skip registering it.
	HealthPath string
}

// Server serves paginated diagram data over HTTP.
type Server struct {
	config *Config

	mu        sync.RWMutex
	metadata  *metadata.Metadata
	lastLoad  time.Time
	cache     map[string]*spec.PaginatedCytoscapeData
	dataCache map[string]*spec.CytoscapeData
}

// PaginatedResponse represents a paginated response.
type PaginatedResponse struct {
	Nodes       []spec.CytoscapeNode `json:"nodes"`
	Edges       []spec.CytoscapeEdge `json:"edges"`
	TotalNodes  int                  `json:"total_nodes"`
	TotalEdges  int                  `json:"total_edges"`
	Page        int                  `json:"page"`
	PageSize    int                  `json:"page_size"`
	HasMore     bool                 `json:"has_more"`
	LoadTime    time.Duration        `json:"load_time_ms"`
	DiagramType string               `json:"diagram_type"`
}

// ErrorResponse represents an error response.
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// PackageNode represents a package in the hierarchy.
type PackageNode struct {
	Name     string        `json:"name"`
	FullPath string        `json:"full_path"`
	Count    int           `json:"count"`
	Children []PackageNode `json:"children,omitempty"`
}

// PackageHierarchyResponse represents the package hierarchy result.
type PackageHierarchyResponse struct {
	RootPackages []PackageNode `json:"root_packages"`
	TotalCount   int           `json:"total_count"`
	DiagramType  string        `json:"diagram_type"`
}

// New constructs a Server with the given config.
func New(config *Config) *Server {
	return &Server{
		config:    config,
		cache:     make(map[string]*spec.PaginatedCytoscapeData),
		dataCache: make(map[string]*spec.CytoscapeData),
	}
}

// SetInputDir changes the project directory and invalidates cached metadata.
// The next request that needs metadata will trigger a fresh LoadMetadata.
func (s *Server) SetInputDir(dir string) {
	s.mu.Lock()
	s.config.InputDir = dir
	s.metadata = nil
	s.cache = make(map[string]*spec.PaginatedCytoscapeData)
	s.dataCache = make(map[string]*spec.CytoscapeData)
	s.mu.Unlock()
}

// LoadMetadata loads and analyzes the Go project at config.InputDir.
func (s *Server) LoadMetadata() error {
	s.mu.Lock()
	dir := s.config.InputDir
	s.mu.Unlock()

	log.Printf("📁 Analyzing project: %s", dir)

	engineConfig := &engine.EngineConfig{
		Verbose:                      s.config.Verbose,
		InputDir:                     dir,
		MaxNodesPerTree:              50000,
		MaxChildrenPerNode:           500,
		MaxArgsPerFunction:           100,
		MaxNestedArgsDepth:           100,
		MaxRecursionDepth:            s.config.MaxDepth,
		SkipCGOPackages:              true,
		AnalyzeFrameworkDependencies: s.config.AnalyzeFrameworkDependencies,
		AutoIncludeFrameworkPackages: s.config.AutoIncludeFrameworkPackages,
		AutoExcludeTests:             s.config.AutoExcludeTests,
		AutoExcludeMocks:             s.config.AutoExcludeMocks,
	}

	genEngine := engine.NewEngine(engineConfig)
	meta, err := genEngine.GenerateMetadataOnly()
	if err != nil {
		return fmt.Errorf("failed to generate metadata: %w", err)
	}

	s.mu.Lock()
	s.metadata = meta
	s.lastLoad = time.Now()
	s.cache = make(map[string]*spec.PaginatedCytoscapeData)
	s.dataCache = make(map[string]*spec.CytoscapeData)
	s.mu.Unlock()

	log.Printf("✅ Metadata loaded successfully")
	if s.config.Verbose {
		log.Printf("📊 Total packages: %d", len(meta.Packages))
		log.Printf("📊 Total call graph edges: %d", len(meta.CallGraph))
	}

	return nil
}

// ensureMetadata lazily loads metadata when a handler needs it.
func (s *Server) ensureMetadata() error {
	s.mu.RLock()
	have := s.metadata != nil
	s.mu.RUnlock()
	if have {
		return nil
	}
	return s.LoadMetadata()
}

// RegisterRoutes mounts the diagram routes on the provided mux.
func (s *Server) RegisterRoutes(mux *http.ServeMux, opts RouteOptions) {
	uiPath := opts.UIPath
	if uiPath == "" {
		uiPath = "/"
	}
	apiPrefix := opts.APIPrefix
	if apiPrefix == "" {
		apiPrefix = "/api/diagram"
	}
	healthPath := opts.HealthPath
	if healthPath == "" {
		healthPath = "/health"
	}

	mux.HandleFunc(uiPath, s.handleIndex)

	// JSON API responses are large and very compressible — wrap them with
	// gzip when the client accepts it.
	mux.Handle(apiPrefix, gzipMiddleware(http.HandlerFunc(s.handleDiagram)))
	mux.Handle(apiPrefix+"/page", gzipMiddleware(http.HandlerFunc(s.handlePaginatedDiagram)))
	mux.Handle(apiPrefix+"/packages", gzipMiddleware(http.HandlerFunc(s.handlePackageHierarchy)))
	mux.Handle(apiPrefix+"/by-packages", gzipMiddleware(http.HandlerFunc(s.handlePackageBasedDiagram)))
	mux.Handle(apiPrefix+"/stats", gzipMiddleware(http.HandlerFunc(s.handleStats)))
	mux.HandleFunc(apiPrefix+"/refresh", s.handleRefresh)
	mux.Handle(apiPrefix+"/export", gzipMiddleware(http.HandlerFunc(s.handleExport)))

	if healthPath != "" {
		mux.HandleFunc(healthPath, s.handleHealth)
	}
}

// --- Gzip middleware -------------------------------------------------------

var gzipPool = sync.Pool{
	New: func() any {
		gw, _ := gzip.NewWriterLevel(io.Discard, gzip.BestSpeed)
		return gw
	},
}

type gzipResponseWriter struct {
	http.ResponseWriter
	gw *gzip.Writer
}

func (g *gzipResponseWriter) Write(b []byte) (int, error) {
	return g.gw.Write(b)
}

// Flush forwards to the underlying writer if it supports flushing.
func (g *gzipResponseWriter) Flush() {
	_ = g.gw.Flush()
	if f, ok := g.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func gzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		gw := gzipPool.Get().(*gzip.Writer)
		gw.Reset(w)
		defer func() {
			_ = gw.Close()
			gzipPool.Put(gw)
		}()

		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Add("Vary", "Accept-Encoding")
		// Content-Length set by the underlying handler would be wrong after
		// compression — drop it.
		w.Header().Del("Content-Length")

		next.ServeHTTP(&gzipResponseWriter{ResponseWriter: w, gw: gw}, r)
	})
}

// --- Handlers --------------------------------------------------------------

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	templateBytes, err := serverUITemplate.ReadFile("server_ui.html")
	if err != nil {
		s.writeError(w, "Failed to load UI template", http.StatusInternalServerError)
		return
	}

	htmlTemplate := string(templateBytes)
	serverURL := fmt.Sprintf("http://%s:%d", s.config.Host, s.config.Port)
	htmlContent := strings.Replace(htmlTemplate, "%s", serverURL, 1)

	s.writeResponse(w, htmlContent, "text/html")
}

func (s *Server) handleDiagram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.ensureMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError)
		return
	}

	start := time.Now()

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

func (s *Server) getAllData(diagramType string, includeFullDepth bool) *spec.CytoscapeData {
	depthKey := "normal"
	if includeFullDepth {
		depthKey = "full"
	}
	cacheKey := fmt.Sprintf("%s:%s", diagramType, depthKey)

	s.mu.RLock()
	if cached, ok := s.dataCache[cacheKey]; ok && cached != nil {
		s.mu.RUnlock()
		return cached
	}
	s.mu.RUnlock()

	var data *spec.CytoscapeData
	if diagramType == "tracker-tree" {
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
		}, nil)
		data = spec.DrawTrackerTreeCytoscapeWithMetadata(trackerTree.GetRoots(), s.metadata)
	} else {
		data = spec.DrawCallGraphCytoscape(s.metadata)
	}

	s.mu.Lock()
	s.dataCache[cacheKey] = data
	s.mu.Unlock()
	return data
}

func (s *Server) handlePaginatedDiagram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.ensureMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError)
		return
	}

	start := time.Now()

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
		depth = s.config.MaxDepth
	} else {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 0 {
			depth = 0
		}
	}

	packages := splitCSV(r.URL.Query().Get("package"))
	functions := splitCSV(r.URL.Query().Get("function"))
	files := splitCSV(r.URL.Query().Get("file"))
	receivers := splitCSV(r.URL.Query().Get("receiver"))
	signatures := splitCSV(r.URL.Query().Get("signature"))
	generics := splitCSV(r.URL.Query().Get("generic"))
	scopeFilter := r.URL.Query().Get("scope")

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

func (s *Server) handlePackageHierarchy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.ensureMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError)
		return
	}

	packageSet := make(map[string]bool)
	packageCounts := make(map[string]int)

	for pkgName := range s.metadata.Packages {
		packageSet[pkgName] = true
		packageCounts[pkgName] = 0
	}

	allData := s.getAllData(s.config.DiagramType, true)

	for _, node := range allData.Nodes {
		pkg := node.Data.Package
		if pkg != "" {
			packageSet[pkg] = true
			packageCounts[pkg]++
		}
	}

	packageMap := make(map[string]map[string]bool)
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

	rootPackages := make(map[string]bool)
	for pkg := range packageSet {
		isRoot := true
		parts := strings.Split(pkg, "/")
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

		if children, exists := packageMap[path]; exists && depth < 20 {
			childPaths := make([]string, 0, len(children))
			for childPath := range children {
				childParts := strings.Split(childPath, "/")
				if len(childParts) == len(parts)+1 {
					childPaths = append(childPaths, childPath)
				}
			}

			sort.Slice(childPaths, func(i, j int) bool {
				return childPaths[i] < childPaths[j]
			})

			node.Children = make([]PackageNode, 0, len(childPaths))
			for _, childPath := range childPaths {
				childNode := buildTree(childPath, depth+1)
				node.Count += childNode.Count
				node.Children = append(node.Children, childNode)
			}
		}

		return node
	}

	rootPaths := make([]string, 0, len(rootPackages))
	for pkg := range rootPackages {
		rootPaths = append(rootPaths, pkg)
	}
	sort.Slice(rootPaths, func(i, j int) bool {
		return rootPaths[i] < rootPaths[j]
	})

	rootNodes := make([]PackageNode, 0, len(rootPaths))
	for _, rootPath := range rootPaths {
		rootNode := buildTree(rootPath, 0)
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

func (s *Server) handlePackageBasedDiagram(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.ensureMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError)
		return
	}

	start := time.Now()

	selectedPackages := r.URL.Query().Get("packages")
	if selectedPackages == "" {
		s.writeError(w, "packages parameter is required", http.StatusBadRequest)
		return
	}

	packages := strings.Split(selectedPackages, ",")
	for i := range packages {
		packages[i] = strings.TrimSpace(packages[i])
	}

	expandedPackages := make(map[string]bool)
	for _, pkg := range packages {
		expandedPackages[pkg] = true
		for pkgName := range s.metadata.Packages {
			if strings.HasPrefix(pkgName, pkg+"/") || pkgName == pkg {
				expandedPackages[pkgName] = true
			}
		}
	}

	finalPackages := make([]string, 0, len(expandedPackages))
	for pkg := range expandedPackages {
		finalPackages = append(finalPackages, pkg)
	}

	depthStr := r.URL.Query().Get("depth")
	var depth int
	if depthStr == "" {
		depth = s.config.MaxDepth
	} else {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 0 {
			depth = 0
		}
	}

	functions := splitCSVTrim(r.URL.Query().Get("function"))
	files := splitCSVTrim(r.URL.Query().Get("file"))
	receivers := splitCSVTrim(r.URL.Query().Get("receiver"))
	signatures := splitCSVTrim(r.URL.Query().Get("signature"))
	generics := splitCSVTrim(r.URL.Query().Get("generic"))
	scopeFilter := r.URL.Query().Get("scope")

	isolate := r.URL.Query().Get("isolate") == "true"

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

func (s *Server) generatePackageBasedData(selectedPackages []string, depth int, functions, files, receivers, signatures, generics []string, scopeFilter string, isolate bool) *spec.PaginatedCytoscapeData {
	allData := s.getAllData(s.config.DiagramType, true)
	if s.config.DiagramType == "tracker-tree" {
		allData.Nodes = spec.OrderTrackerTreeNodesDepthFirst(allData)
	}

	var depthFilteredNodes []spec.CytoscapeNode
	var depthFilteredEdges []spec.CytoscapeEdge

	if s.config.DiagramType == "call-graph" && depth >= 0 {
		nodeDepths := s.calculateCallGraphDepth(allData)

		nodeIDSet := make(map[string]bool)
		for _, node := range allData.Nodes {
			nodeDepth, hasDepth := nodeDepths[node.Data.ID]
			if hasDepth && nodeDepth <= depth {
				depthFilteredNodes = append(depthFilteredNodes, node)
				nodeIDSet[node.Data.ID] = true
			}
		}

		for _, edge := range allData.Edges {
			if nodeIDSet[edge.Data.Source] && nodeIDSet[edge.Data.Target] {
				depthFilteredEdges = append(depthFilteredEdges, edge)
			}
		}
	} else {
		depthFilteredNodes = allData.Nodes
		depthFilteredEdges = allData.Edges
	}

	var filteredNodes []spec.CytoscapeNode
	for _, node := range depthFilteredNodes {
		pkg := node.Data.Package
		matchPackage := false

		for _, selectedPkg := range selectedPackages {
			if pkg == selectedPkg || strings.HasPrefix(pkg, selectedPkg+"/") {
				matchPackage = true
				break
			}
		}

		if !matchPackage {
			continue
		}

		if !nodeMatchesFilters(node, functions, files, receivers, signatures, generics, scopeFilter) {
			continue
		}

		filteredNodes = append(filteredNodes, node)
	}

	nodeIDs := make(map[string]bool)
	for _, node := range filteredNodes {
		nodeIDs[node.Data.ID] = true
	}

	var filteredEdges []spec.CytoscapeEdge

	if isolate {
		for _, edge := range depthFilteredEdges {
			if nodeIDs[edge.Data.Source] && nodeIDs[edge.Data.Target] {
				filteredEdges = append(filteredEdges, edge)
			}
		}
	} else {
		connectedNodeIDs := make(map[string]bool)

		for _, edge := range depthFilteredEdges {
			sourceInSet := nodeIDs[edge.Data.Source]
			targetInSet := nodeIDs[edge.Data.Target]

			if sourceInSet && targetInSet {
				filteredEdges = append(filteredEdges, edge)
			} else if sourceInSet || targetInSet {
				connectedID := edge.Data.Source
				if sourceInSet {
					connectedID = edge.Data.Target
				}

				for _, node := range allData.Nodes {
					if node.Data.ID == connectedID {
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

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := s.ensureMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError)
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

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	log.Printf("🔄 Refreshing metadata...")

	if err := s.LoadMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to refresh metadata: %v", err), http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"message":   "Metadata refreshed successfully",
		"timestamp": time.Now().Format(time.RFC3339),
	}

	s.writeJSON(w, response)
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	format := r.URL.Query().Get("format")
	if format == "" {
		format = "svg"
	}

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

	if err := s.ensureMetadata(); err != nil {
		s.writeError(w, fmt.Sprintf("Failed to load metadata: %v", err), http.StatusInternalServerError)
		return
	}

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
		depth = s.config.MaxDepth
	} else {
		depth, _ = strconv.Atoi(depthStr)
		if depth < 0 {
			depth = 0
		}
	}

	packages := splitCSV(r.URL.Query().Get("package"))
	functions := splitCSV(r.URL.Query().Get("function"))
	files := splitCSV(r.URL.Query().Get("file"))
	receivers := splitCSV(r.URL.Query().Get("receiver"))
	signatures := splitCSV(r.URL.Query().Get("signature"))
	generics := splitCSV(r.URL.Query().Get("generic"))
	scopeFilter := r.URL.Query().Get("scope")

	data := s.generatePaginatedData(page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=\"diagram.%s\"", format))

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	switch format {
	case "json":
		jsonData, err := json.MarshalIndent(data, "", "  ")
		if err != nil {
			s.writeError(w, "Failed to generate JSON export", http.StatusInternalServerError)
			return
		}
		if _, err := w.Write(jsonData); err != nil {
			log.Printf("Failed to write JSON export: %v", err)
		}
		return

	default:
		message := fmt.Sprintf("Format '%s' is now handled client-side using Cytoscape.js extensions. Please use the export dropdown in the UI.", format)
		s.writeError(w, message, http.StatusBadRequest)
		return
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
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

// --- Graph helpers ---------------------------------------------------------

func (s *Server) calculateCallGraphDepth(data *spec.CytoscapeData) map[string]int {
	depths := make(map[string]int)

	adjacency := make(map[string][]string)
	inDegree := make(map[string]int)

	for _, node := range data.Nodes {
		inDegree[node.Data.ID] = 0
		adjacency[node.Data.ID] = []string{}
	}

	for _, edge := range data.Edges {
		adjacency[edge.Data.Source] = append(adjacency[edge.Data.Source], edge.Data.Target)
		inDegree[edge.Data.Target]++
	}

	var roots []string
	for _, node := range data.Nodes {
		if inDegree[node.Data.ID] == 0 || node.Data.Label == "main" || node.Data.FunctionName == "main" {
			roots = append(roots, node.Data.ID)
		}
	}

	if len(roots) == 0 {
		for nodeID, degree := range inDegree {
			if degree == 0 {
				roots = append(roots, nodeID)
			}
		}
	}

	type queueEntry struct {
		nodeID string
		depth  int
	}
	queue := make([]queueEntry, 0)

	for _, root := range roots {
		queue = append(queue, queueEntry{nodeID: root, depth: 0})
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

		for _, neighbor := range adjacency[current.nodeID] {
			newDepth := current.depth + 1

			if existingDepth, exists := depths[neighbor]; !exists || newDepth < existingDepth {
				depths[neighbor] = newDepth
				queue = append(queue, queueEntry{nodeID: neighbor, depth: newDepth})
			}
		}
	}

	return depths
}

func (s *Server) generatePaginatedData(page, pageSize, depth int, packages, functions, files, receivers, signatures, generics []string, scopeFilter string) *spec.PaginatedCytoscapeData {
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

func (s *Server) generatePaginatedDataInternal(page, pageSize, depth int, packages, functions, files, receivers, signatures, generics []string, scopeFilter string) *spec.PaginatedCytoscapeData {
	cacheKey := fmt.Sprintf("%s-%d-%d-%d-%v-%v-%v-%v-%v-%v-%s", s.config.DiagramType, page, pageSize, depth, packages, functions, files, receivers, signatures, generics, scopeFilter)

	s.mu.RLock()
	if cached, exists := s.cache[cacheKey]; exists {
		s.mu.RUnlock()
		return cached
	}
	s.mu.RUnlock()

	allData := s.getAllData(s.config.DiagramType, true)
	if s.config.DiagramType == "tracker-tree" {
		allData.Nodes = spec.OrderTrackerTreeNodesDepthFirst(allData)
	}

	allNodes := make(map[string]*spec.CytoscapeNode)
	for i := range allData.Nodes {
		node := allData.Nodes[i]
		allNodes[node.Data.ID] = &node
	}

	var depthFilteredNodes []spec.CytoscapeNode
	var depthFilteredEdges []spec.CytoscapeEdge

	if s.config.DiagramType == "call-graph" && depth >= 0 {
		nodeDepths := s.calculateCallGraphDepth(allData)

		if s.config.Verbose {
			log.Printf("Calculated depths for %d nodes, filtering to depth %d", len(nodeDepths), depth)
		}

		nodeIDSet := make(map[string]bool)
		for _, node := range allData.Nodes {
			nodeDepth, hasDepth := nodeDepths[node.Data.ID]
			if hasDepth && nodeDepth <= depth {
				depthFilteredNodes = append(depthFilteredNodes, node)
				nodeIDSet[node.Data.ID] = true
			}
		}

		for _, edge := range allData.Edges {
			if nodeIDSet[edge.Data.Source] && nodeIDSet[edge.Data.Target] {
				depthFilteredEdges = append(depthFilteredEdges, edge)
			}
		}
	} else {
		depthFilteredNodes = allData.Nodes
		depthFilteredEdges = allData.Edges
	}

	var filteredNodes []spec.CytoscapeNode
	var filteredEdges []spec.CytoscapeEdge

	for _, node := range depthFilteredNodes {
		if len(packages) > 0 {
			packageMatch := false
			for _, pkg := range packages {
				if strings.Contains(node.Data.Package, strings.TrimSpace(pkg)) {
					packageMatch = true
					break
				}
			}
			if !packageMatch {
				continue
			}
		}

		if !nodeMatchesFilters(node, functions, files, receivers, signatures, generics, scopeFilter) {
			continue
		}

		filteredNodes = append(filteredNodes, node)
	}

	nodeIDs := make(map[string]*spec.CytoscapeNode)
	for i := range filteredNodes {
		node := filteredNodes[i]
		nodeIDs[node.Data.ID] = &node
	}

	for _, edge := range depthFilteredEdges {
		if nodeIDs[edge.Data.Source] != nil && nodeIDs[edge.Data.Target] != nil {
			filteredEdges = append(filteredEdges, edge)
		}
	}

	var paginatedNodes []spec.CytoscapeNode
	var start, end int
	if s.config.DiagramType == "tracker-tree" {
		filteredData := &spec.CytoscapeData{Nodes: filteredNodes, Edges: filteredEdges}
		orderedNodes := spec.TraverseTrackerTreeBranchOrder(filteredData)

		start = (page - 1) * pageSize
		end = start + pageSize
		if start < len(orderedNodes) {
			if end > len(orderedNodes) {
				end = len(orderedNodes)
			}
			paginatedNodes = orderedNodes[start:end]
		}
	} else {
		start = (page - 1) * pageSize
		end = start + pageSize
		if start < len(filteredNodes) {
			if end > len(filteredNodes) {
				end = len(filteredNodes)
			}
			paginatedNodes = filteredNodes[start:end]
		}
	}

	paginatedNodeIDs := make(map[string]*spec.CytoscapeNode)
	for i := range paginatedNodes {
		node := paginatedNodes[i]
		paginatedNodeIDs[node.Data.ID] = &node
	}

	var paginatedEdges []spec.CytoscapeEdge
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

	s.mu.Lock()
	s.cache[cacheKey] = result
	s.mu.Unlock()

	return result
}

// --- Small utilities -------------------------------------------------------

func splitCSV(raw string) []string {
	parts := strings.Split(raw, ",")
	if len(parts) == 1 && parts[0] == "" {
		return []string{}
	}
	return parts
}

func splitCSVTrim(raw string) []string {
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func matchesFunctionName(functionName, searchTerm string) bool {
	if searchTerm == "" {
		return false
	}
	return strings.Contains(strings.ToLower(functionName), strings.ToLower(strings.TrimSpace(searchTerm)))
}

func nodeMatchesFilters(node spec.CytoscapeNode, functions, files, receivers, signatures, generics []string, scopeFilter string) bool {
	if len(functions) > 0 {
		match := false
		for _, function := range functions {
			if matchesFunctionName(node.Data.Label, function) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(files) > 0 {
		match := false
		if node.Data.Position != "" {
			for _, file := range files {
				if strings.Contains(strings.ToLower(node.Data.Position), strings.ToLower(strings.TrimSpace(file))) {
					match = true
					break
				}
			}
		}
		if !match && len(node.Data.CallPaths) > 0 {
			for _, file := range files {
				for _, callPath := range node.Data.CallPaths {
					if strings.Contains(strings.ToLower(callPath.Position), strings.ToLower(strings.TrimSpace(file))) {
						match = true
						break
					}
				}
				if match {
					break
				}
			}
		}
		if !match {
			return false
		}
	}

	if len(receivers) > 0 && node.Data.ReceiverType != "" {
		match := false
		for _, receiver := range receivers {
			if strings.Contains(strings.ToLower(node.Data.ReceiverType), strings.ToLower(strings.TrimSpace(receiver))) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(signatures) > 0 && node.Data.SignatureStr != "" {
		match := false
		for _, signature := range signatures {
			if strings.Contains(strings.ToLower(node.Data.SignatureStr), strings.ToLower(strings.TrimSpace(signature))) {
				match = true
				break
			}
		}
		if !match {
			return false
		}
	}

	if len(generics) > 0 && node.Data.Generics != nil {
		match := false
		for _, genericFilter := range generics {
			for _, generic := range node.Data.Generics {
				if strings.Contains(strings.ToLower(generic), strings.ToLower(strings.TrimSpace(genericFilter))) {
					match = true
					break
				}
			}
			if match {
				break
			}
		}
		if !match {
			return false
		}
	}

	if scopeFilter != "" && scopeFilter != "all" {
		nodeScope := strings.ToLower(strings.TrimSpace(node.Data.Scope))
		switch strings.ToLower(strings.TrimSpace(scopeFilter)) {
		case "exported":
			if nodeScope != "exported" {
				return false
			}
		case "unexported":
			if nodeScope != "unexported" {
				return false
			}
		}
	}

	return true
}

// --- Response writers ------------------------------------------------------

func (s *Server) writeJSON(w http.ResponseWriter, data interface{}) {
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

func (s *Server) writeResponse(w http.ResponseWriter, data string, contentType string) {
	w.Header().Set("Content-Type", contentType)

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	if _, err := w.Write([]byte(data)); err != nil {
		log.Printf("Failed to write response: %v", err)
	}
}

func (s *Server) writeError(w http.ResponseWriter, message string, code int) {
	w.Header().Set("Content-Type", "application/json")

	if s.config.EnableCORS {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
	}

	w.WriteHeader(code)

	errorResp := ErrorResponse{
		Error:   http.StatusText(code),
		Message: message,
		Code:    code,
	}

	if err := json.NewEncoder(w).Encode(errorResp); err != nil {
		log.Printf("Failed to encode error response: %v", err)
	}
}
