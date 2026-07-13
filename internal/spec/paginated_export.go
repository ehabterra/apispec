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

package spec

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

//go:embed paginated_template.html
var paginatedTemplate embed.FS

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
