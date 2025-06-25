package core

import (
	"go/ast"
	"go/token"
	"go/types"
)

// RouteStrategy defines the interface for different route parsing strategies
type RouteStrategy interface {
	TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next RouteStrategy, info *types.Info) (*ParsedRoute, bool)
}

type StrategyChain struct {
	strategies []RouteStrategy
}

func (c *StrategyChain) tryParseAt(i int, node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, info *types.Info) (*ParsedRoute, bool) {
	if i >= len(c.strategies) {
		return nil, false
	}
	return c.strategies[i].TryParse(node, fset, funcMap, goFiles, &strategyChainNext{c, i + 1}, info)
}

type strategyChainNext struct {
	chain *StrategyChain
	index int
}

func (n *strategyChainNext) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, _ RouteStrategy, info *types.Info) (*ParsedRoute, bool) {
	return n.chain.tryParseAt(n.index, node, fset, funcMap, goFiles, info)
}

// NewStrategyChain creates a new strategy chain
func NewStrategyChain(strategies ...RouteStrategy) *StrategyChain {
	return &StrategyChain{strategies: strategies}
}

// TryParse attempts to parse using each strategy in the chain
func (c *StrategyChain) TryParse(node ast.Node, fset *token.FileSet, funcMap map[string]*ast.FuncDecl, goFiles []*ast.File, next RouteStrategy, info *types.Info) (*ParsedRoute, bool) {
	for _, strategy := range c.strategies {
		if route, ok := strategy.TryParse(node, fset, funcMap, goFiles, next, info); ok {
			return route, true
		}
	}
	return nil, false
}
