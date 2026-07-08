// Package main is a robustness regression fixture for self-referential and
// mutually-recursive types.
//
// Historical failure: issue #10 (stack overflow — cyclic struct drove
// unbounded recursion in generateStructSchema) and issue #14 (truncated output
// on the same project). The schema mapper must break every type cycle by
// emitting a $ref back to the already-registered component instead of expanding
// the type inline forever. This fixture wires three distinct cycle shapes into
// HTTP responses so a regression surfaces as either a hang/stack-overflow (test
// never returns) or a dangling/missing component ($ref assertions fail).
package main

import (
	"encoding/json"
	"net/http"
)

// TreeNode is DIRECTLY self-referential through two different field kinds: a
// pointer back to the parent and a slice of pointers to children. Both edges
// must terminate as a $ref to TreeNode.
type TreeNode struct {
	ID       int         `json:"id"`
	Value    string      `json:"value"`
	Parent   *TreeNode   `json:"parent,omitempty"`
	Children []*TreeNode `json:"children,omitempty"`
}

// Category and Product are MUTUALLY recursive: a category lists its products,
// each product points back at its category, and a category also nests under a
// parent category. Two interleaved cycles (Category<->Product and
// Category->Category) must both close as $refs.
type Category struct {
	Name     string    `json:"name"`
	Parent   *Category `json:"parent,omitempty"`
	Products []Product `json:"products"`
}

type Product struct {
	SKU      string    `json:"sku"`
	Category *Category `json:"category"`
	Related  []Product `json:"related,omitempty"`
}

// Graph, Edge and Node form a THREE-hop cycle (Graph -> Edge -> Node -> Graph)
// to exercise a cycle that only closes after several levels of nesting.
type Graph struct {
	Root  *Node  `json:"root"`
	Edges []Edge `json:"edges"`
}

type Edge struct {
	From *Node `json:"from"`
	To   *Node `json:"to"`
}

type Node struct {
	Label string `json:"label"`
	Graph *Graph `json:"graph,omitempty"`
}

func getTree(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(TreeNode{})
}

func getCategory(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Category{})
}

func getGraph(w http.ResponseWriter, r *http.Request) {
	_ = json.NewEncoder(w).Encode(Graph{})
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/tree", getTree)
	mux.HandleFunc("/category", getCategory)
	mux.HandleFunc("/graph", getGraph)
	_ = http.ListenAndServe(":8080", mux)
}
