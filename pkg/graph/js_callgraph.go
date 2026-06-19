package graph

import (
	"context"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	ts "github.com/smacker/go-tree-sitter/typescript/typescript"
)

func (cg *CallGraph) ParseJSAST(filePath, src, pkg string) error {
	p := sitter.NewParser()
	p.SetLanguage(jsGraphLang(filePath))

	tree, err := p.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		return err
	}

	srcBytes := []byte(src)
	root := tree.RootNode()

	cg.mu.Lock()
	defer cg.mu.Unlock()

	for i := 0; i < int(root.ChildCount()); i++ {
		jsTopLevel(cg, root.Child(i), srcBytes, filePath, pkg)
	}
	return nil
}

func jsTopLevel(cg *CallGraph, node *sitter.Node, src []byte, file, pkg string) {
	switch node.Type() {
	case "function_declaration", "generator_function_declaration":
		jsAddFunc(cg, node, src, file, pkg, "")
	case "class_declaration":
		jsAddClass(cg, node, src, file, pkg)
	case "lexical_declaration", "variable_statement":
		jsAddVarFuncs(cg, node, src, file, pkg)
	case "export_statement":
		for i := 0; i < int(node.ChildCount()); i++ {
			jsTopLevel(cg, node.Child(i), src, file, pkg)
		}
	case "internal_module", "module":
		jsWalkNamespace(cg, node, src, file, pkg)

	case "expression_statement":
		if node.ChildCount() > 0 {
			jsTopLevel(cg, node.Child(0), src, file, pkg)
		}
	}
}

func jsWalkNamespace(cg *CallGraph, node *sitter.Node, src []byte, file, pkg string) {
	body := jsFirstChild(node, "statement_block")
	if body == nil {
		return
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		jsTopLevel(cg, body.Child(i), src, file, pkg)
	}
}

func jsAddFunc(cg *CallGraph, node *sitter.Node, src []byte, file, pkg, recv string) {
	name := jsNodeChildText(node, src, "identifier", "property_identifier")
	if name == "" {
		return
	}
	n := &Node{Pkg: pkg, Name: name, Recv: recv, File: file, Line: int(node.StartPoint().Row) + 1}
	cg.AddNode(n)
	jsWalkCalls(cg, node, src, file, pkg, recv, name, n.ID())
}

func jsAddClass(cg *CallGraph, node *sitter.Node, src []byte, file, pkg string) {
	className := jsNodeChildText(node, src, "type_identifier", "identifier")
	if className == "" {
		return
	}
	body := jsFirstChild(node, "class_body")
	if body == nil {
		return
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		child := body.Child(i)
		if child.Type() == "method_definition" {
			jsAddFunc(cg, child, src, file, pkg, className)
		}
	}
}

func jsAddVarFuncs(cg *CallGraph, node *sitter.Node, src []byte, file, pkg string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		decl := node.Child(i)
		if decl.Type() != "variable_declarator" {
			continue
		}
		name := jsNodeChildText(decl, src, "identifier")
		if name == "" {
			continue
		}
		val := jsFirstChild(decl, "arrow_function", "function", "generator_function")
		if val == nil {
			continue
		}
		n := &Node{Pkg: pkg, Name: name, File: file, Line: int(node.StartPoint().Row) + 1}
		cg.AddNode(n)
		jsWalkCalls(cg, val, src, file, pkg, "", name, n.ID())
	}
}

func jsWalkCalls(cg *CallGraph, node *sitter.Node, src []byte, file, pkg, recv, name, callerID string) {
	if node.Type() == "call_expression" {
		fn := node.ChildByFieldName("function")
		if fn == nil {
			fn = jsFirstChild(node, "identifier", "member_expression")
		}
		if fn != nil {
			calleeID := jsResolveCallee(fn, src, pkg, recv)
			if calleeID != "" && calleeID != callerID {
				cg.AddEdge(callerID, calleeID, int(node.StartPoint().Row)+1, file)
			}
		}
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		jsWalkCalls(cg, node.Child(i), src, file, pkg, recv, name, callerID)
	}
}

func jsResolveCallee(fn *sitter.Node, src []byte, pkg, recv string) string {
	switch fn.Type() {
	case "identifier":
		name := string(fn.Content(src))
		if name == "" {
			return ""
		}
		return pkg + "." + name

	case "member_expression":
		obj := fn.Child(0)
		prop := jsFirstChild(fn, "property_identifier")
		if obj == nil || prop == nil {
			return ""
		}
		propName := string(prop.Content(src))
		switch obj.Type() {
		case "this":
			if recv != "" {
				return pkg + ".(" + recv + ")." + propName
			}
		case "super":
			return ""
		}
		return jsMemberChain(fn, src)
	}
	return ""
}

func jsMemberChain(node *sitter.Node, src []byte) string {
	if node.Type() == "identifier" || node.Type() == "this" {
		return string(node.Content(src))
	}
	if node.Type() == "member_expression" {
		obj := node.Child(0)
		prop := jsFirstChild(node, "property_identifier")
		if obj == nil || prop == nil {
			return string(node.Content(src))
		}
		left := jsMemberChain(obj, src)
		right := string(prop.Content(src))
		if left == "" {
			return right
		}
		return left + "." + right
	}
	return string(node.Content(src))
}

func jsNodeChildText(node *sitter.Node, src []byte, types ...string) string {
	set := make(map[string]bool, len(types))
	for _, t := range types {
		set[t] = true
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if set[child.Type()] {
			return string(child.Content(src))
		}
	}
	return ""
}

func jsFirstChild(node *sitter.Node, types ...string) *sitter.Node {
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

func jsGraphLang(filePath string) *sitter.Language {
	switch filepath.Ext(filePath) {
	case ".ts":
		return ts.GetLanguage()
	case ".tsx":
		return tsx.GetLanguage()
	default:
		return javascript.GetLanguage()
	}
}
