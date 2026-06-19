package instrument

import (
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
)

type Instrumenter struct {
	tracerPkg string
}

func NewInstrumenter() *Instrumenter {
	return &Instrumenter{
		tracerPkg: "dataflow",
	}
}

func (inst *Instrumenter) Instrument(src, filePath, pkg, varFilter string) (string, error) {
	if inst.skipFile(filePath) {
		return src, nil
	}

	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return "", fmt.Errorf("parse: %w", err)
	}

	needsFmt := false

	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			return true
		}

		funcID := pkg + "." + fn.Name.Name
		fileBase := filePath
		if idx := strings.LastIndex(filePath, "/"); idx >= 0 {
			fileBase = filePath[idx+1:]
		}

		entryCall := inst.makeEventCall("entry", funcID, fileBase, fset.Position(fn.Pos()).Line)
		exitCall := &ast.DeferStmt{
			Call: inst.makeEventCallExpr("exit", funcID, fileBase, fset.Position(fn.End()).Line),
		}

		stmtList := fn.Body.List
		newBody := make([]ast.Stmt, 0, len(stmtList)+10)
		newBody = append(newBody, &ast.ExprStmt{X: entryCall})
		newBody = append(newBody, exitCall)

		for _, stmt := range stmtList {
			newBody = append(newBody, stmt)

			if assign, ok := stmt.(*ast.AssignStmt); ok && (assign.Tok == token.DEFINE || assign.Tok == token.ASSIGN) {
				for _, lhs := range assign.Lhs {
					if ident, ok := lhs.(*ast.Ident); ok && ident.Name != "_" {
						assignCall := inst.makeAssignCall(ident.Name, ident, fileBase, fset.Position(assign.Pos()).Line)
						newBody = append(newBody, &ast.ExprStmt{X: assignCall})
						needsFmt = true
					}
				}
			}
		}

		fn.Body.List = newBody
		return true
	})

	if needsFmt && !hasImport(f, "fmt") {
		addImport(f, "fmt")
	}

	var buf strings.Builder
	if err := format.Node(&buf, fset, f); err != nil {
		return "", fmt.Errorf("format: %w", err)
	}

	return buf.String(), nil
}

func (inst *Instrumenter) skipFile(filePath string) bool {
	if strings.Contains(filePath, "pkg/instrument/") {
		return true
	}
	if strings.Contains(filePath, "pkg/dataflow/") {
		return true
	}
	return false
}

func (inst *Instrumenter) makeEventCall(eventType, funcID, file string, line int) ast.Expr {
	args := []ast.Expr{
		stringLit(eventType),
		stringLit(funcID),
		stringLit(file),
		intLit(line),
	}
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: "__trace_event"},
		Args: args,
	}
}

func (inst *Instrumenter) makeEventCallExpr(eventType, funcID, file string, line int) *ast.CallExpr {
	args := []ast.Expr{
		stringLit(eventType),
		stringLit(funcID),
		stringLit(file),
		intLit(line),
	}
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: "__trace_event"},
		Args: args,
	}
}

func (inst *Instrumenter) makeAssignCall(varName string, varExpr ast.Expr, file string, line int) ast.Expr {
	sprintCall := &ast.CallExpr{
		Fun: &ast.SelectorExpr{
			X:   &ast.Ident{Name: "fmt"},
			Sel: &ast.Ident{Name: "Sprint"},
		},
		Args: []ast.Expr{varExpr},
	}

	args := []ast.Expr{
		stringLit("assign"),
		stringLit(varName),
		sprintCall,
		stringLit(file),
		intLit(line),
	}
	return &ast.CallExpr{
		Fun:  &ast.Ident{Name: "__trace_event"},
		Args: args,
	}
}

func stringLit(s string) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.STRING, Value: `"` + s + `"`}
}

func intLit(n int) *ast.BasicLit {
	return &ast.BasicLit{Kind: token.INT, Value: fmt.Sprintf("%d", n)}
}

func hasImport(f *ast.File, pkg string) bool {
	for _, imp := range f.Imports {
		if strings.Trim(imp.Path.Value, `"`) == pkg {
			return true
		}
	}
	return false
}

func addImport(f *ast.File, pkg string) {
	spec := &ast.ImportSpec{
		Path: &ast.BasicLit{Kind: token.STRING, Value: `"` + pkg + `"`},
	}
	f.Imports = append(f.Imports, spec)

	for _, decl := range f.Decls {
		if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.IMPORT {
			genDecl.Specs = append(genDecl.Specs, spec)
			return
		}
	}

	importDecl := &ast.GenDecl{
		Tok:   token.IMPORT,
		Specs: []ast.Spec{spec},
	}
	f.Decls = append([]ast.Decl{importDecl}, f.Decls...)
}
