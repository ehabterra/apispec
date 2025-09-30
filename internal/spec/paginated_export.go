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
	_ = depth // Use depth to avoid ineffassign

	packageFilter := r.URL.Query().Get("package")

	// Calculate pagination
	start := (page - 1) * s.pageSize
	end := start + s.pageSize

	// Get paginated data
	paginatedData := s.getPaginatedData(start, end, packageFilter)

	// Set headers
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	// Return JSON
	err := json.NewEncoder(w).Encode(paginatedData)
	if err != nil {
		http.Error(w, "Failed to write error response", http.StatusInternalServerError)
		return
	}
}

// getPaginatedData returns a subset of the call graph data
func (s *PaginatedCallGraphServer) getPaginatedData(start, end int, packageFilter string) *PaginatedCytoscapeData {
	// Apply package filtering first
	var filteredNodes []CytoscapeNode
	var filteredEdges []CytoscapeEdge

	if packageFilter != "" {
		// Filter nodes by package
		for _, node := range s.allData.Nodes {
			if strings.Contains(node.Data.Package, packageFilter) {
				filteredNodes = append(filteredNodes, node)
			}
		}

		// Filter edges to only include those between filtered nodes
		nodeIDs := make(map[string]bool)
		for _, node := range filteredNodes {
			nodeIDs[node.Data.ID] = true
		}

		for _, edge := range s.allData.Edges {
			if nodeIDs[edge.Data.Source] && nodeIDs[edge.Data.Target] {
				filteredEdges = append(filteredEdges, edge)
			}
		}
	} else {
		filteredNodes = s.allData.Nodes
		filteredEdges = s.allData.Edges
	}

	// Apply pagination with proper edge handling
	var paginatedNodes []CytoscapeNode
	if start < len(filteredNodes) {
		if end > len(filteredNodes) {
			end = len(filteredNodes)
		}
		paginatedNodes = filteredNodes[start:end]
	}

	// Create a map of paginated node IDs for quick lookup
	paginatedNodeIDs := make(map[string]bool)
	for _, node := range paginatedNodes {
		paginatedNodeIDs[node.Data.ID] = true
	}

	// Only include edges that connect paginated nodes
	var paginatedEdges []CytoscapeEdge
	for _, edge := range filteredEdges {
		if paginatedNodeIDs[edge.Data.Source] && paginatedNodeIDs[edge.Data.Target] {
			paginatedEdges = append(paginatedEdges, edge)
		}
	}

	return &PaginatedCytoscapeData{
		Nodes:      paginatedNodes,
		Edges:      paginatedEdges,
		TotalNodes: len(filteredNodes),
		TotalEdges: len(filteredEdges),
		Page:       (start / s.pageSize) + 1,
		PageSize:   s.pageSize,
		HasMore:    end < len(filteredNodes),
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
