package parser_test

import (
	"go/ast"
	goparser "go/parser"
	"go/token"
	"testing"

	"github.com/ehabterra/swagen/internal/parser"
)

const ginExample = `
package main
import "github.com/gin-gonic/gin"
func main() {
	r := gin.Default()
	r.GET("/hello", helloHandler)
}
func helloHandler(c *gin.Context) {}
`

func TestGinParser_Parse(t *testing.T) {
	fset := token.NewFileSet()
	file, err := goparser.ParseFile(fset, "example.go", ginExample, goparser.ParseComments)
	if err != nil {
		t.Fatalf("failed to parse example: %v", err)
	}

	p := parser.DefaultGinParser()
	routes, err := p.Parse(fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}
	if len(routes) != 1 {
		t.Fatalf("expected 1 route, got %d", len(routes))
	}
	r := routes[0]
	if r.Method != "GET" || r.Path != "/hello" || r.HandlerName != "helloHandler" {
		t.Errorf("unexpected route: %+v", r)
	}
}
