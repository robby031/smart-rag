package indexer

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

type VariableRef struct {
	Name  string `json:"name"`
	Type  string `json:"type,omitempty"`
	IsDef bool   `json:"is_def"`
	Line  int    `json:"line"`
}

type VariableExtractor struct{}

func NewVariableExtractor() *VariableExtractor {
	return &VariableExtractor{}
}

func (ve *VariableExtractor) ExtractVariables(decl ParsedDecl, src string, pkg string) []VariableRef {
	if decl.StartLine == 0 || decl.EndLine == 0 || src == "" {
		return nil
	}

	fset := token.NewFileSet()
	wrapped := "package " + pkg + "\n" + decl.Content
	f, err := parser.ParseFile(fset, "", wrapped, parser.ParseComments)
	if err != nil {
		return nil
	}

	e := &dataflow.DefUseExtractor{}
	chains, err := e.ExtractDefUse(f, fset, "", pkg)
	if err != nil {
		return nil
	}

	lineOffset := decl.StartLine - 1
	contentLines := strings.Split(decl.Content, "\n")
	firstContentLine := 1

	var refs []VariableRef
	seen := make(map[string]bool)

	for _, chain := range chains {
		defLine := chain.Def.StartLine
		if defLine == 0 {
			defLine = chain.Def.EndLine
		}
		if defLine < firstContentLine || defLine > len(contentLines) {
			continue
		}

		actualLine := defLine + lineOffset
		defKey := chain.Def.Variable + ":def"
		if !seen[defKey] {
			ref := VariableRef{
				Name:  chain.Def.Variable,
				IsDef: true,
				Line:  actualLine,
			}
			refs = append(refs, ref)
			seen[defKey] = true
		}

		for _, use := range chain.Uses {
			useLine := use.Line
			if useLine < firstContentLine || useLine > len(contentLines) {
				continue
			}
			useKey := fmt.Sprintf("%s:use:%d", use.Variable, use.Line)
			if !seen[useKey] {
				refs = append(refs, VariableRef{
					Name:  use.Variable,
					IsDef: false,
					Line:  useLine + lineOffset,
				})
				seen[useKey] = true
			}
		}
	}

	refs = ve.extractParamTypes(refs, f)

	return refs
}

func (ve *VariableExtractor) extractParamTypes(refs []VariableRef, f *ast.File) []VariableRef {
	paramTypes := make(map[string]string)
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Type.Params == nil {
			return true
		}
		for _, field := range fn.Type.Params.List {
			typeStr := typeName(field.Type)
			for _, name := range field.Names {
				paramTypes[name.Name] = typeStr
			}
		}
		return true
	})

	for i, ref := range refs {
		if ref.IsDef {
			if t, ok := paramTypes[ref.Name]; ok {
				refs[i].Type = t
			}
		}
	}
	return refs
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
	case *ast.SelectorExpr:
		return typeName(e.X) + "." + e.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeName(e.Elt)
	case *ast.MapType:
		return "map[" + typeName(e.Key) + "]" + typeName(e.Value)
	case *ast.IndexExpr:
		return typeName(e.X) + "[" + typeName(e.Index) + "]"
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
