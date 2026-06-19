package indexer

import (
	"context"
	"path/filepath"
	"strings"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	ts "github.com/smacker/go-tree-sitter/typescript/typescript"
)

func IsJSLike(filePath string) bool {
	switch filepath.Ext(filePath) {
	case ".js", ".jsx", ".mjs", ".cjs", ".ts", ".tsx":
		return true
	}
	return false
}

func ParseJSFile(filePath, src string) ([]ParsedDecl, FileInfo, error) {
	p := sitter.NewParser()
	p.SetLanguage(jsLangForPath(filePath))

	srcBytes := []byte(src)
	tree, err := p.ParseCtx(context.Background(), nil, srcBytes)
	if err != nil {
		return nil, FileInfo{}, err
	}

	root := tree.RootNode()

	var decls []ParsedDecl
	var imports []string

	for i := 0; i < int(root.ChildCount()); i++ {
		child := root.Child(i)
		newDecls, imp := jsVisitNode(child, srcBytes)
		decls = append(decls, newDecls...)
		if imp != "" {
			imports = append(imports, imp)
		}
	}

	base := filepath.Base(filePath)
	pkg := strings.TrimSuffix(base, filepath.Ext(base))
	isTest := strings.Contains(base, ".test.") || strings.Contains(base, ".spec.")

	return decls, FileInfo{
		Package: pkg,
		Imports: imports,
		IsTest:  isTest,
	}, nil
}

func jsLangForPath(filePath string) *sitter.Language {
	switch filepath.Ext(filePath) {
	case ".ts":
		return ts.GetLanguage()
	case ".tsx":
		return tsx.GetLanguage()
	default:
		return javascript.GetLanguage()
	}
}

func jsVisitNode(node *sitter.Node, src []byte) ([]ParsedDecl, string) {
	switch node.Type() {
	case "import_statement":
		return nil, jsExtractImportPath(node, src)

	case "function_declaration", "generator_function_declaration":
		if d, ok := jsParseFuncDecl(node, src); ok {
			return []ParsedDecl{d}, ""
		}

	case "class_declaration":
		if d, ok := jsParseClassDecl(node, src); ok {
			return []ParsedDecl{d}, ""
		}

	case "interface_declaration":
		if d, ok := jsParseInterfaceDecl(node, src); ok {
			return []ParsedDecl{d}, ""
		}

	case "type_alias_declaration":
		if d, ok := jsParseTypeAliasDecl(node, src); ok {
			return []ParsedDecl{d}, ""
		}

	case "enum_declaration":
		if d, ok := jsParseEnumDecl(node, src); ok {
			return []ParsedDecl{d}, ""
		}

	case "lexical_declaration", "variable_statement":
		return jsParseVarDecl(node, src), ""

	case "export_statement":
		return jsUnwrapExport(node, src)

	case "internal_module", "module":
		return jsWalkNamespaceBody(node, src)

	case "expression_statement":
		if node.ChildCount() > 0 {
			return jsVisitNode(node.Child(0), src)
		}
	}

	return nil, ""
}

func jsWalkNamespaceBody(node *sitter.Node, src []byte) ([]ParsedDecl, string) {
	body := jsFindChild(node, "statement_block")
	if body == nil {
		return nil, ""
	}
	var decls []ParsedDecl
	for i := 0; i < int(body.ChildCount()); i++ {
		d, _ := jsVisitNode(body.Child(i), src)
		decls = append(decls, d...)
	}
	return decls, ""
}

func jsUnwrapExport(node *sitter.Node, src []byte) ([]ParsedDecl, string) {
	var decls []ParsedDecl
	var importPath string

	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		switch child.Type() {
		case "function_declaration", "generator_function_declaration":
			if d, ok := jsParseFuncDecl(child, src); ok {
				decls = append(decls, d)
			}
		case "class_declaration":
			if d, ok := jsParseClassDecl(child, src); ok {
				decls = append(decls, d)
			}
		case "interface_declaration":
			if d, ok := jsParseInterfaceDecl(child, src); ok {
				decls = append(decls, d)
			}
		case "type_alias_declaration":
			if d, ok := jsParseTypeAliasDecl(child, src); ok {
				decls = append(decls, d)
			}
		case "enum_declaration":
			if d, ok := jsParseEnumDecl(child, src); ok {
				decls = append(decls, d)
			}
		case "lexical_declaration", "variable_statement":
			decls = append(decls, jsParseVarDecl(child, src)...)
		case "string":
			importPath = jsUnquote(string(child.Content(src)))
		}
	}

	return decls, importPath
}

func jsParseFuncDecl(node *sitter.Node, src []byte) (ParsedDecl, bool) {
	name := jsChildContent(node, src, "identifier")
	if name == "" {
		name = "<default>"
	}
	return ParsedDecl{
		Kind:      DeclFunc,
		Name:      name,
		Signature: jsBuildSig(node, src),
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	}, true
}

func jsParseClassDecl(node *sitter.Node, src []byte) (ParsedDecl, bool) {
	name := jsChildContent(node, src, "type_identifier")
	if name == "" {
		name = jsChildContent(node, src, "identifier")
	}
	if name == "" {
		name = "<default>"
	}
	return ParsedDecl{
		Kind:      DeclClass,
		Name:      name,
		Signature: jsBuildSig(node, src),
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	}, true
}

func jsParseInterfaceDecl(node *sitter.Node, src []byte) (ParsedDecl, bool) {
	name := jsChildContent(node, src, "type_identifier")
	if name == "" {
		return ParsedDecl{}, false
	}
	return ParsedDecl{
		Kind:      DeclInterface,
		Name:      name,
		Signature: jsBuildSig(node, src),
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	}, true
}

func jsParseTypeAliasDecl(node *sitter.Node, src []byte) (ParsedDecl, bool) {
	name := jsChildContent(node, src, "type_identifier")
	if name == "" {
		return ParsedDecl{}, false
	}
	sig := strings.TrimSpace(string(node.Content(src)))
	if len(sig) > 200 {
		sig = sig[:200] + "…"
	}
	return ParsedDecl{
		Kind:      DeclType,
		Name:      name,
		Signature: sig,
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	}, true
}

func jsParseEnumDecl(node *sitter.Node, src []byte) (ParsedDecl, bool) {
	name := jsChildContent(node, src, "identifier")
	if name == "" {
		return ParsedDecl{}, false
	}
	return ParsedDecl{
		Kind:      DeclEnum,
		Name:      name,
		Signature: jsBuildSig(node, src),
		StartLine: int(node.StartPoint().Row) + 1,
		EndLine:   int(node.EndPoint().Row) + 1,
	}, true
}

func jsParseVarDecl(node *sitter.Node, src []byte) []ParsedDecl {
	var decls []ParsedDecl
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "variable_declarator" {
			continue
		}
		name := jsChildContent(child, src, "identifier")
		if name == "" {
			continue
		}
		val := jsFindChild(child, "arrow_function", "function", "generator_function")
		if val == nil {
			continue
		}
		decls = append(decls, ParsedDecl{
			Kind:      DeclFunc,
			Name:      name,
			Signature: jsBuildSig(node, src),
			StartLine: int(node.StartPoint().Row) + 1,
			EndLine:   int(node.EndPoint().Row) + 1,
		})
	}
	return decls
}

func jsExtractImportPath(node *sitter.Node, src []byte) string {
	for i := int(node.ChildCount()) - 1; i >= 0; i-- {
		child := node.Child(i)
		if child.Type() == "string" {
			return jsUnquote(string(child.Content(src)))
		}
	}
	return ""
}

func jsBuildSig(node *sitter.Node, src []byte) string {
	full := string(node.Content(src))
	if idx := strings.Index(full, "{"); idx > 0 {
		full = full[:idx]
	}
	sig := strings.Join(strings.Fields(full), " ")
	if len(sig) > 200 {
		return sig[:200] + "…"
	}
	return sig
}

func jsChildContent(node *sitter.Node, src []byte, typ string) string {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() == typ {
			return string(child.Content(src))
		}
	}
	return ""
}

func jsFindChild(node *sitter.Node, types ...string) *sitter.Node {
	set := make(map[string]bool, len(types))
	for _, t := range types {
		set[t] = true
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		if child := node.Child(i); set[child.Type()] {
			return child
		}
	}
	return nil
}

func jsUnquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 && (s[0] == '\'' || s[0] == '"' || s[0] == '`') {
		return s[1 : len(s)-1]
	}
	return s
}
