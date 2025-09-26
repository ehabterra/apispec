package spec

import (
	"embed"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

//go:embed paginated_template.html
var paginatedTemplate embed.FS

//go:embed server_template.html
var serverTemplate embed.FS

// PaginatedCytoscapeData represents paginated data for Cytoscape.js
type PaginatedCytoscapeData struct {
	Nodes      []CytoscapeNode `json:"nodes"`
	Edges      []CytoscapeEdge `json:"edges"`
	TotalNodes int             `json:"total_nodes"`
	TotalEdges int             `json:"total_edges"`
	Page       int             `json:"page"`
	PageSize   int             `json:"page_size"`
	HasMore    bool            `json:"has_more"`
}

// PaginatedCallGraphServer creates an HTTP server for serving paginated call graph data
type PaginatedCallGraphServer struct {
	meta     *metadata.Metadata
	allData  *CytoscapeData
	pageSize int
}

// NewPaginatedCallGraphServer creates a new paginated server
func NewPaginatedCallGraphServer(meta *metadata.Metadata, pageSize int) *PaginatedCallGraphServer {
	// Pre-generate all data once
	allData := DrawCallGraphCytoscape(meta)

	return &PaginatedCallGraphServer{
		meta:     meta,
		allData:  allData,
		pageSize: pageSize,
	}
}

// ServeHTTP implements http.Handler
func (s *PaginatedCallGraphServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Parse query parameters
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page < 1 {
		page = 1
	}

	depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
	if depth < 1 {
		depth = 2 // Default depth
	}

	packageFilter := r.URL.Query().Get("package")

	// Calculate pagination
	start := (page - 1) * s.pageSize
	end := start + s.pageSize

	// Get paginated data
	paginatedData := s.getPaginatedData(start, end, depth, packageFilter)

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Return JSON
	json.NewEncoder(w).Encode(paginatedData)
}

// getPaginatedData returns a subset of the call graph data
func (s *PaginatedCallGraphServer) getPaginatedData(start, end, depth int, packageFilter string) *PaginatedCytoscapeData {
	// For now, return a simple slice of the data
	// In a real implementation, you'd want to implement proper pagination logic

	var nodes []CytoscapeNode
	var edges []CytoscapeEdge

	// Simple pagination - in practice, you'd want smarter pagination
	if start < len(s.allData.Nodes) {
		if end > len(s.allData.Nodes) {
			end = len(s.allData.Nodes)
		}
		nodes = s.allData.Nodes[start:end]
	}

	if start < len(s.allData.Edges) {
		if end > len(s.allData.Edges) {
			end = len(s.allData.Edges)
		}
		edges = s.allData.Edges[start:end]
	}

	return &PaginatedCytoscapeData{
		Nodes:      nodes,
		Edges:      edges,
		TotalNodes: len(s.allData.Nodes),
		TotalEdges: len(s.allData.Edges),
		Page:       (start / s.pageSize) + 1,
		PageSize:   s.pageSize,
		HasMore:    end < len(s.allData.Nodes),
	}
}

// GeneratePaginatedCytoscapeHTML generates HTML with pagination support
func GeneratePaginatedCytoscapeHTML(meta *metadata.Metadata, outputPath string, pageSize int) error {
	// Generate all data first
	allData := DrawCallGraphCytoscape(meta)

	// Convert to JSON
	jsonData, err := json.MarshalIndent(allData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cytoscape data: %w", err)
	}

	// Read the embedded template
	templateBytes, err := paginatedTemplate.ReadFile("paginated_template.html")
	if err != nil {
		return fmt.Errorf("failed to read paginated template: %w", err)
	}

	// Replace the placeholder with actual data
	htmlTemplate := string(templateBytes)
	htmlContent := strings.Replace(htmlTemplate, "%s", string(jsonData), 1)

	// Write the HTML file
	err = os.WriteFile(outputPath, []byte(htmlContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write HTML file: %w", err)
	}

	return nil
}

// GenerateServerBasedCytoscapeHTML generates HTML that connects to a diagram server
func GenerateServerBasedCytoscapeHTML(serverURL, outputPath string) error {
	// Read the embedded template
	templateBytes, err := serverTemplate.ReadFile("server_template.html")
	if err != nil {
		return fmt.Errorf("failed to read server template: %w", err)
	}

	// Replace the placeholder with actual server URL
	htmlTemplate := string(templateBytes)
	htmlContent := strings.Replace(htmlTemplate, "%s", serverURL, 1)

	// Write the HTML file
	err = os.WriteFile(outputPath, []byte(htmlContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write HTML file: %w", err)
	}

	return nil
}
