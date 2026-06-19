package dataflow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

// CallGraphLookup provides call graph lookup without direct dependency on graph package.
type CallGraphLookup interface {
	HasNode(id string) bool
}

type FlowGraphBuilder struct {
	typeTracker *TypeFlowTracker
	callGraph   CallGraphLookup
	funcChains  map[string][]*DefUseChain
}

func NewFlowGraphBuilder(cg CallGraphLookup) *FlowGraphBuilder {
	return &FlowGraphBuilder{
		typeTracker: NewTypeFlowTracker(),
		callGraph:   cg,
		funcChains:  make(map[string][]*DefUseChain),
	}
}

func BuildFlowFromSource(src, filePath, pkg string, cg CallGraphLookup) (*FlowGraph, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}
	b := NewFlowGraphBuilder(cg)
	return b.BuildFromAST(f, fset, filePath, pkg)
}

func (b *FlowGraphBuilder) BuildFromAST(astFile *ast.File, fset *token.FileSet, filePath, pkg string) (*FlowGraph, error) {
	extractor := &DefUseExtractor{}
	chains, err := extractor.ExtractDefUse(astFile, fset, filePath, pkg)
	if err != nil {
		return nil, fmt.Errorf("extract defuse: %w", err)
	}

	if err := b.typeTracker.ExtractFromAST(astFile, fset, pkg); err != nil {
		return nil, fmt.Errorf("extract typeflow: %w", err)
	}

	// Store chains by funcID
	for _, chain := range chains {
		funcID := chain.Def.Pkg + "." + extractor.lastFuncID(chain)
		b.funcChains[funcID] = append(b.funcChains[funcID], chain)
	}

	fg := b.buildGraph(chains, astFile, fset, filePath, pkg)

	edges := b.buildEdges(chains, astFile, fset, filePath, pkg)
	fg.Edges = edges

	return fg, nil
}

func (b *FlowGraphBuilder) buildGraph(chains []*DefUseChain, _ *ast.File, _ *token.FileSet, _ string, _ string) *FlowGraph {
	fg := &FlowGraph{
		Variables: make(map[string]*Variable),
		Defs:      make(map[string]*VarDef),
		Uses:      make(map[string]*VarUse),
		DefUseMap: make(map[string]*DefUseChain),
		TypeNodes: b.typeTracker.nodes,
		Edges:     nil,
	}

	for _, chain := range chains {
		def := chain.Def
		fg.Defs[def.ID] = &def
		fg.DefUseMap[def.ID] = chain

		if _, exists := fg.Variables[def.Variable]; !exists {
			fg.Variables[def.Variable] = &Variable{
				Name:    def.Variable,
				Pkg:     def.Pkg,
				File:    def.File,
				DefLine: def.StartLine,
			}
		}

		for _, use := range chain.Uses {
			u := use
			fg.Uses[u.ID] = &u
		}
	}

	return fg
}

func (b *FlowGraphBuilder) buildEdges(chains []*DefUseChain, astFile *ast.File, fset *token.FileSet, filePath, pkg string) []DataFlowEdge {
	var edges []DataFlowEdge

	edges = append(edges, b.buildIntraEdges(chains)...)
	edges = append(edges, b.buildInterEdges(astFile, fset, filePath, pkg)...)

	return edges
}

func (b *FlowGraphBuilder) buildIntraEdges(chains []*DefUseChain) []DataFlowEdge {
	var edges []DataFlowEdge
	for _, chain := range chains {
		for _, use := range chain.Uses {
			kind := "def_use"
			if use.Kind == UseCallArg {
				kind = "call_arg"
			} else if use.Kind == UseReturn {
				kind = "return_assign"
			}
			edges = append(edges, DataFlowEdge{
				From: chain.Def.ID,
				To:   use.ID,
				Kind: kind,
				File: use.File,
				Line: use.Line,
			})
		}
	}
	return edges
}

func (b *FlowGraphBuilder) buildInterEdges(astFile *ast.File, fset *token.FileSet, filePath, pkg string) []DataFlowEdge {
	var edges []DataFlowEdge

	ast.Inspect(astFile, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		callPos := fset.Position(call.Pos())

		calleeID := extractCallName(call)
		if calleeID == "" {
			return true
		}

		fullCallee := pkg + "." + calleeID
		if b.callGraph != nil && b.callGraph.HasNode(fullCallee) {
			for i, arg := range call.Args {
				if ident, ok := arg.(*ast.Ident); ok {
					edges = append(edges, DataFlowEdge{
						From: fmt.Sprintf("%s.%s:%d:%s", pkg, filePath, callPos.Line, ident.Name),
						To:   fmt.Sprintf("%s.param_%d", fullCallee, i),
						Kind: "call_arg",
						File: filePath,
						Line: callPos.Line,
					})
				}
			}
		}

		return true
	})

	return edges
}

func (b *DefUseExtractor) lastFuncID(chain *DefUseChain) string {
	for _, use := range chain.Uses {
		if use.FuncID != "" {
			return use.FuncID
		}
	}
	return b.pkg
}

func extractCallName(call *ast.CallExpr) string {
	return selectorChain(call.Fun)
}

func selectorChain(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		x := selectorChain(e.X)
		if x == "" {
			return e.Sel.Name
		}
		return x + "." + e.Sel.Name
	case *ast.IndexExpr:
		return selectorChain(e.X)
	case *ast.FuncLit:
		return "<anonymous>"
	default:
		return ""
	}
}
