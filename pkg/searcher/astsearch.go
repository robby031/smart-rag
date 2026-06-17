package searcher

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
)

type MatchResult struct {
	FilePath string `json:"file_path"`
	Line     int    `json:"line"`
	Snippet  string `json:"snippet"`
	NodeType string `json:"node_type"`
	Name     string `json:"name,omitempty"`
}

type ASTSearch struct{}

func NewASTSearch() *ASTSearch {
	return &ASTSearch{}
}

func (s *ASTSearch) FindFuncBySignature(filePath, src, pattern string) ([]MatchResult, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	var results []MatchResult

	for _, decl := range f.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}

		if matchesFunction(fn, pattern) {
			pos := fset.Position(fn.Pos())
			snippet := extractSnippet(src, pos.Line)
			name := fn.Name.Name
			if fn.Recv != nil && len(fn.Recv.List) > 0 {
				recv := receiverStr(fn.Recv.List[0].Type)
				name = fmt.Sprintf("(%s).%s", recv, name)
			}
			results = append(results, MatchResult{
				FilePath: filePath,
				Line:     pos.Line,
				Snippet:  snippet,
				NodeType: "function",
				Name:     name,
			})
		}
	}

	return results, nil
}

func (s *ASTSearch) FindType(filePath, src, pattern string) ([]MatchResult, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	var results []MatchResult

	for _, decl := range f.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}

		for _, spec := range gen.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}

			if pattern == "*" || ts.Name.Name == pattern || strings.Contains(ts.Name.Name, pattern) {
				pos := fset.Position(ts.Pos())
				snippet := extractSnippet(src, pos.Line)

				nodeType := "type"
				switch ts.Type.(type) {
				case *ast.StructType:
					nodeType = "struct"
				case *ast.InterfaceType:
					nodeType = "interface"
				}

				results = append(results, MatchResult{
					FilePath: filePath,
					Line:     pos.Line,
					Snippet:  snippet,
					NodeType: nodeType,
					Name:     ts.Name.Name,
				})
			}
		}
	}

	return results, nil
}

func (s *ASTSearch) FindReferences(filePath, src, symbol string) ([]MatchResult, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filePath, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", filePath, err)
	}

	var results []MatchResult

	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.Ident:
			if node.Name == symbol {
				pos := fset.Position(node.Pos())
				snippet := extractSnippet(src, pos.Line)
				results = append(results, MatchResult{
					FilePath: filePath,
					Line:     pos.Line,
					Snippet:  snippet,
					NodeType: "reference",
					Name:     symbol,
				})
			}
		}
		return true
	})

	return results, nil
}

func WalkFiles(root string, maxFiles int) ([]string, error) {
	var files []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if maxFiles > 0 && len(files) >= maxFiles {
			return filepath.SkipDir
		}
		if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.Contains(path, "/vendor/") && !strings.Contains(path, "/.") {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func matchesFunction(fn *ast.FuncDecl, pattern string) bool {
	if pattern == "*" {
		return true
	}

	if strings.HasPrefix(pattern, "func ") {
		namePart := strings.TrimPrefix(pattern, "func ")
		if strings.HasPrefix(namePart, "(") {
			parts := strings.SplitN(namePart, ").", 2)
			if len(parts) == 2 {
				recvPattern := strings.TrimPrefix(parts[0], "(")
				methodPattern := parts[1]
				if fn.Recv == nil || len(fn.Recv.List) == 0 {
					return false
				}
				actualRecv := receiverStr(fn.Recv.List[0].Type)
				return (recvPattern == "*" || recvPattern == actualRecv) &&
					(methodPattern == "*" || fn.Name.Name == methodPattern)
			}
		}
		return fn.Name.Name == namePart || namePart == "*"
	}

	return fn.Name.Name == pattern || strings.Contains(fn.Name.Name, pattern)
}

func receiverStr(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + receiverStr(t.X)
	default:
		return "?"
	}
}

func extractSnippet(src string, line int) string {
	lines := strings.Split(src, "\n")
	if line-1 < 0 || line-1 >= len(lines) {
		return ""
	}
	return strings.TrimSpace(lines[line-1])
}
