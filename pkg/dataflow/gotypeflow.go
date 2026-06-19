package dataflow

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
)

type TypeFlowTracker struct {
	nodes map[string]*TypeFlowNode
}

func NewTypeFlowTracker() *TypeFlowTracker {
	return &TypeFlowTracker{
		nodes: make(map[string]*TypeFlowNode),
	}
}

func ExtractTypeFlowFromSource(src, filePath, pkg string) ([]*TypeFlowNode, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse source: %w", err)
	}
	t := NewTypeFlowTracker()
	if err := t.ExtractFromAST(f, fset, pkg); err != nil {
		return nil, err
	}
	nodes := make([]*TypeFlowNode, 0, len(t.nodes))
	for _, n := range t.nodes {
		nodes = append(nodes, n)
	}
	return nodes, nil
}

func (t *TypeFlowTracker) ExtractFromAST(astFile *ast.File, fset *token.FileSet, pkg string) error {
	if t.nodes == nil {
		t.nodes = make(map[string]*TypeFlowNode)
	}

	ast.Inspect(astFile, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.TypeSpec:
			t.processTypeSpec(node, fset, pkg)
		case *ast.FuncDecl:
			t.processFuncDecl(node, fset, pkg)
			return false
		}
		return true
	})

	return nil
}

func (t *TypeFlowTracker) processTypeSpec(spec *ast.TypeSpec, fset *token.FileSet, _ string) {
	name := spec.Name.Name
	pos := fset.Position(spec.Pos())

	node := t.getOrCreate(name)
	node.DefFile = pos.Filename
	node.DefLine = pos.Line

	if st, ok := spec.Type.(*ast.StructType); ok {
		for _, field := range st.Fields.List {
			ft := typeName(field.Type)
			if ft == "" {
				continue
			}

			if len(field.Names) == 0 {
				embedded := t.getOrCreate(ft)
				embedded.UsedAsField = append(embedded.UsedAsField, name)
			} else {
				fieldType := t.getOrCreate(ft)
				fieldType.UsedAsField = append(fieldType.UsedAsField, name)
			}
		}
	}
}

func (t *TypeFlowTracker) processFuncDecl(fn *ast.FuncDecl, _ *token.FileSet, pkg string) {
	funcID := fmt.Sprintf("%s.%s", pkg, fn.Name.Name)

	if fn.Type.Params != nil {
		for _, field := range fn.Type.Params.List {
			paramTypeName := typeName(field.Type)
			if paramTypeName == "" {
				continue
			}
			pt := t.getOrCreate(paramTypeName)
			pt.UsedAsParam = append(pt.UsedAsParam, funcID)
		}
	}

	if fn.Type.Results != nil {
		for _, field := range fn.Type.Results.List {
			resultTypeName := typeName(field.Type)
			if resultTypeName == "" {
				continue
			}
			rt := t.getOrCreate(resultTypeName)
			rt.UsedAsReturn = append(rt.UsedAsReturn, funcID)
		}
	}
}

func (t *TypeFlowTracker) getOrCreate(typeName string) *TypeFlowNode {
	if n, ok := t.nodes[typeName]; ok {
		return n
	}
	n := &TypeFlowNode{
		TypeName: typeName,
	}
	t.nodes[typeName] = n
	return n
}

func (t *TypeFlowTracker) BuildTypeGraph() []DataFlowEdge {
	var edges []DataFlowEdge
	for _, node := range t.nodes {
		for _, field := range node.UsedAsField {
			edges = append(edges, DataFlowEdge{
				From: node.TypeName,
				To:   field,
				Kind: "type_field",
			})
		}
	}
	return edges
}

func typeName(expr ast.Expr) string {
	if expr == nil {
		return ""
	}
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.StarExpr:
		return "*" + typeName(e.X)
	case *ast.ArrayType:
		return "[]" + typeName(e.Elt)
	case *ast.SelectorExpr:
		return typeName(e.X) + "." + e.Sel.Name
	case *ast.IndexExpr:
		return typeName(e.X) + "[" + typeName(e.Index) + "]"
	case *ast.MapType:
		return "map[" + typeName(e.Key) + "]" + typeName(e.Value)
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.FuncType:
		return "func"
	case *ast.StructType:
		return "struct"
	case *ast.Ellipsis:
		return "..." + typeName(e.Elt)
	default:
		return ""
	}
}
