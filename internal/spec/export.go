package spec

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/ehabterra/apispec/internal/metadata"
)

//go:embed cytoscape_template.html
var cytoscapeTemplate embed.FS

const (
	cytoscapeTemplateFile = "cytoscape_template.html"
	jsonIndentPrefix      = ""
	jsonIndent            = "  "
	htmlDataPlaceholder   = "%s"
	htmlFilePerm          = 0644
	errorMarshalCytoscape = "failed to marshal cytoscape data: %w"
	errorReadHTMLTemplate = "failed to read HTML template: %w"
	errorWriteHTMLFile    = "failed to write HTML file: %w"
	errorWriteJSONFile    = "failed to write JSON file: %w"
)

// GenerateCytoscapeHTML generates an HTML file with Cytoscape.js visualization.
// The HTML template is loaded from cytoscape_template.html in the same directory.
func GenerateCytoscapeHTML(nodes []TrackerNodeInterface, outputPath string) error {
	cytoscapeData := DrawTrackerTreeCytoscape(nodes)
	jsonData, err := json.MarshalIndent(cytoscapeData, jsonIndentPrefix, jsonIndent)
	if err != nil {
		return fmt.Errorf(errorMarshalCytoscape, err)
	}

	templateBytes, err := cytoscapeTemplate.ReadFile(cytoscapeTemplateFile)
	if err != nil {
		return fmt.Errorf(errorReadHTMLTemplate, err)
	}
	htmlTemplate := string(templateBytes)
	htmlContent := strings.Replace(htmlTemplate, htmlDataPlaceholder, string(jsonData), 1)
	err = os.WriteFile(outputPath, []byte(htmlContent), htmlFilePerm)
	if err != nil {
		return fmt.Errorf(errorWriteHTMLFile, err)
	}
	return nil
}

// ExportCytoscapeJSON exports Cytoscape data as JSON file.
func ExportCytoscapeJSON(nodes []TrackerNodeInterface, outputPath string) error {
	cytoscapeData := DrawTrackerTreeCytoscape(nodes)
	jsonData, err := json.MarshalIndent(cytoscapeData, jsonIndentPrefix, jsonIndent)
	if err != nil {
		return fmt.Errorf(errorMarshalCytoscape, err)
	}
	err = os.WriteFile(outputPath, jsonData, htmlFilePerm)
	if err != nil {
		return fmt.Errorf(errorWriteJSONFile, err)
	}
	return nil
}

// GenerateCallGraphCytoscapeHTML generates an HTML file with Cytoscape.js visualization using call graph data.
func GenerateCallGraphCytoscapeHTML(meta *metadata.Metadata, outputPath string) error {
	cytoscapeData := DrawCallGraphCytoscape(meta)
	jsonData, err := json.MarshalIndent(cytoscapeData, jsonIndentPrefix, jsonIndent)
	if err != nil {
		return fmt.Errorf(errorMarshalCytoscape, err)
	}

	templateBytes, err := cytoscapeTemplate.ReadFile(cytoscapeTemplateFile)
	if err != nil {
		return fmt.Errorf(errorReadHTMLTemplate, err)
	}
	htmlTemplate := string(templateBytes)
	htmlContent := strings.Replace(htmlTemplate, htmlDataPlaceholder, string(jsonData), 1)
	err = os.WriteFile(outputPath, []byte(htmlContent), htmlFilePerm)
	if err != nil {
		return fmt.Errorf(errorWriteHTMLFile, err)
	}
	return nil
}

// ExportCallGraphCytoscapeJSON exports call graph Cytoscape data as JSON file.
func ExportCallGraphCytoscapeJSON(meta *metadata.Metadata, outputPath string) error {
	cytoscapeData := DrawCallGraphCytoscape(meta)
	jsonData, err := json.MarshalIndent(cytoscapeData, jsonIndentPrefix, jsonIndent)
	if err != nil {
		return fmt.Errorf(errorMarshalCytoscape, err)
	}
	err = os.WriteFile(outputPath, jsonData, htmlFilePerm)
	if err != nil {
		return fmt.Errorf(errorWriteJSONFile, err)
	}
	return nil
}

// GenerateOptimizedCallGraphHTML generates an optimized HTML file for large call graphs
func GenerateOptimizedCallGraphHTML(meta *metadata.Metadata, outputPath string, optimizationType string) error {
	switch optimizationType {
	case "paginated":
		return GeneratePaginatedCytoscapeHTML(meta, outputPath, 100)
	default:
		// Fallback to original implementation
		return GenerateCallGraphCytoscapeHTML(meta, outputPath)
	}
}
