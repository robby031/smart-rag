package graph

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

func (cg *CallGraph) ParseFile(filePath, src string, pkg string) error {
	f, err := parser.ParseFile(cg.Fset, filePath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filePath, err)
	}
	return cg.ParseAST(f, cg.Fset, filePath, pkg)
}

func (cg *CallGraph) ParseAST(f *ast.File, fset *token.FileSet, filePath, pkg string) error {
	cg.mu.Lock()
	defer cg.mu.Unlock()
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			cg.processFuncDecl(node, fset, filePath, pkg)
			return false
		case *ast.CallExpr:
			cg.processCallExpr(node, fset, filePath)
		}
		return true
	})
	return nil
}

func (cg *CallGraph) processFuncDecl(fn *ast.FuncDecl, fset *token.FileSet, filePath, pkg string) {
	pos := fset.Position(fn.Pos())
	recv := ""
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv = receiverType(fn.Recv.List[0].Type)
	}
	node := &Node{
		Pkg:  pkg,
		Name: fn.Name.Name,
		Recv: recv,
		File: filePath,
		Line: pos.Line,
	}
	cg.AddNode(node)
	callerID := node.ID()

	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			calleeID := extractCallName(call)
			if calleeID != "" && calleeID != callerID {
				callPos := fset.Position(call.Pos())
				cg.AddEdge(callerID, calleeID, callPos.Line, filePath)
			}
			return false
		})
	}
}

func (cg *CallGraph) processCallExpr(call *ast.CallExpr, fset *token.FileSet, filePath string) {
	calleeID := extractCallName(call)
	if calleeID != "" {
		pos := fset.Position(call.Pos())
		cg.AddEdge(":<package-init>", calleeID, pos.Line, filePath)
	}
}
