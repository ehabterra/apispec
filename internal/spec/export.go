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
)

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
