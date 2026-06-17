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
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if id, ok := fun.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", id.Name, fun.Sel.Name)
		}
		return fun.Sel.Name
	case *ast.IndexExpr:
		if id, ok := fun.X.(*ast.Ident); ok {
			return id.Name
		}
		return ""
	case *ast.FuncLit:
		return "<anonymous>"
	default:
		return ""
	}
}
