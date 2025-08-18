package spec

import (
	"fmt"
	"strings"
)

const (
	mermaidGraphHeader = "graph LR\n"
	nodePrefix         = "node_"
	edgePrefix         = "edge_"
)

// DrawTrackerTree generates a Mermaid graph for the tracker tree.
func DrawTrackerTree(nodes []*TrackerNode) string {
	var str = strings.Builder{}
	var counter = 0
	str.WriteString(mermaidGraphHeader)
	for _, node := range nodes {
		drawNode(node, &str, &counter)
	}
	return str.String()
}

func drawNode(node *TrackerNode, str *strings.Builder, counter *int) {
	nodeID := fmt.Sprintf("%s%d", nodePrefix, *counter)
	for _, child := range node.Children {
		*counter++
		fmt.Fprintf(str, "  %s[%q] --> %s[%q]\n", nodeID, node.Key(), fmt.Sprintf("%s%d", nodePrefix, *counter), child.Key())
		drawNode(child, str, counter)
	}
}

// CytoscapeData represents the data structure for Cytoscape.js
// and related node/edge types.
type CytoscapeData struct {
	Nodes []CytoscapeNode `json:"nodes"`
	Edges []CytoscapeEdge `json:"edges"`
}

type CytoscapeNode struct {
	Data CytoscapeNodeData `json:"data"`
}

type CytoscapeNodeData struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Parent   string `json:"parent,omitempty"`
	Group    string `json:"group,omitempty"`
	Position string `json:"position,omitempty"`
	Type     string `json:"type,omitempty"`
}

type CytoscapeEdge struct {
	Data CytoscapeEdgeData `json:"data"`
}

type CytoscapeEdgeData struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
	Label  string `json:"label,omitempty"`
	Type   string `json:"type,omitempty"`
}

// DrawTrackerTreeCytoscape generates Cytoscape.js JSON data for the tracker tree.
func DrawTrackerTreeCytoscape(nodes []*TrackerNode) *CytoscapeData {
	data := &CytoscapeData{
		Nodes: make([]CytoscapeNode, 0),
		Edges: make([]CytoscapeEdge, 0),
	}
	nodeMap := make(map[string]string)
	edgeCounter := 0
	nodeCounter := 0
	for _, node := range nodes {
		drawNodeCytoscape(node, data, nodeMap, &edgeCounter, &nodeCounter)
	}
	return data
}

func drawNodeCytoscape(node *TrackerNode, data *CytoscapeData, nodeMap map[string]string, edgeCounter, nodeCounter *int) {
	if node == nil {
		return
	}
	nodeID := fmt.Sprintf("%s%d", nodePrefix, *nodeCounter)
	data.Nodes = append(data.Nodes, CytoscapeNode{
		Data: CytoscapeNodeData{
			ID:    nodeID,
			Label: node.Key(),
			Type:  "function",
		},
	})
	for _, child := range node.Children {
		if child != nil {
			*nodeCounter++
			childID := fmt.Sprintf("%s%d", nodePrefix, *nodeCounter)
			nodeMap[child.Key()] = childID
			data.Nodes = append(data.Nodes, CytoscapeNode{
				Data: CytoscapeNodeData{
					ID:    nodeMap[child.Key()],
					Label: child.Key(),
					Type:  "function",
				},
			})
			edgeID := fmt.Sprintf("%s%d", edgePrefix, *edgeCounter)
			*edgeCounter++
			data.Edges = append(data.Edges, CytoscapeEdge{
				Data: CytoscapeEdgeData{
					ID:     edgeID,
					Source: nodeID,
					Target: childID,
					Type:   "calls",
				},
			})
			drawNodeCytoscape(child, data, nodeMap, edgeCounter, nodeCounter)
		}
	}
}
