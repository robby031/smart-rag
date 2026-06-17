package graph

import (
	"fmt"
	"go/ast"
)

func receiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + receiverType(t.X)
	case *ast.IndexExpr:
		return receiverType(t.X) + "[" + receiverType(t.Index) + "]"
	default:
		return fmt.Sprintf("%T", expr)
	}
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
