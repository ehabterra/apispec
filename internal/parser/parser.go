package parser

import (
	"go/ast"
	"go/token"

	"github.com/ehabterra/swagen/internal/core"
)

type Parser interface {
	Parse(fset *token.FileSet, files []*ast.File) ([]core.ParsedRoute, error)
}
