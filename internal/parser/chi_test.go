package parser

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"
)

const chiExample = `
package main
import "github.com/go-chi/chi/v5"
func main() {
	r := chi.NewRouter()
	r.Get("/hello", helloHandler)
}
func helloHandler(w http.ResponseWriter, r *http.Request) {}
`

func TestChiParser_Parse(t *testing.T) {
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, "example.go", chiExample, goparser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse example: %v", err)
	}

	p := DefaultChiParser()
	routes, err := p.Parse(fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	r := routes[0]
	if r.Method != "Get" || r.Path != "/hello" || r.HandlerName != "helloHandler" {
		t.Errorf("unexpected route: %+v", r)
	}
}
