package spec

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

//go:embed cytoscape_template.html
var cytoscapeTemplate embed.FS

// GenerateCytoscapeHTML generates an HTML file with Cytoscape.js visualization.
// The HTML template is loaded from cytoscape_template.html in the same directory.
func GenerateCytoscapeHTML(nodes []*TrackerNode, outputPath string) error {
	cytoscapeData := DrawTrackerTreeCytoscape(nodes)
	jsonData, err := json.MarshalIndent(cytoscapeData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cytoscape data: %w", err)
	}

	templateBytes, err := cytoscapeTemplate.ReadFile("cytoscape_template.html")
	if err != nil {
		return fmt.Errorf("failed to read HTML template: %w", err)
	}
	htmlTemplate := string(templateBytes)
	htmlContent := strings.Replace(htmlTemplate, "%s", string(jsonData), 1)
	err = os.WriteFile(outputPath, []byte(htmlContent), 0644)
	if err != nil {
		return fmt.Errorf("failed to write HTML file: %w", err)
	}
	return nil
}

// ExportCytoscapeJSON exports Cytoscape data as JSON file.
func ExportCytoscapeJSON(nodes []*TrackerNode, outputPath string) error {
	cytoscapeData := DrawTrackerTreeCytoscape(nodes)
	jsonData, err := json.MarshalIndent(cytoscapeData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cytoscape data: %w", err)
	}
	err = os.WriteFile(outputPath, jsonData, 0644)
	if err != nil {
		return fmt.Errorf("failed to write JSON file: %w", err)
	}
	return nil
}
