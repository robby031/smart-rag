package dataflow

import (
	"context"
	"fmt"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	"github.com/smacker/go-tree-sitter/javascript"
	"github.com/smacker/go-tree-sitter/typescript/tsx"
	ts "github.com/smacker/go-tree-sitter/typescript/typescript"
)

type JSDefUseExtractor struct {
	variables  map[string]*Variable
	defs       []VarDef
	uses       []VarUse
	defRefs    map[string]string
	scopeStack []map[string]VarDef
	pkg        string
	file       string
	funcID     string
	funcPkg    string
	funcFile   string
	src        []byte
}

func NewJSDefUseExtractor() *JSDefUseExtractor {
	return &JSDefUseExtractor{}
}

func (e *JSDefUseExtractor) ExtractDefUse(src, filePath, pkg string) ([]*DefUseChain, error) {
	lang := jsLang(filePath)
	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, []byte(src))
	if err != nil {
		return nil, fmt.Errorf("parse js: %w", err)
	}

	return e.ExtractDefUseFromNode(tree.RootNode(), []byte(src), filePath, pkg)
}

func (e *JSDefUseExtractor) ExtractDefUseFromNode(root *sitter.Node, src []byte, filePath, pkg string) ([]*DefUseChain, error) {
	e.src = src
	e.pkg = pkg
	e.file = filePath
	e.funcPkg = pkg
	e.funcFile = filePath
	e.variables = make(map[string]*Variable)
	e.defRefs = make(map[string]string)
	e.defs = nil
	e.uses = nil
	e.scopeStack = nil
	e.funcID = ""

	e.pushScope()

	for i := 0; i < int(root.ChildCount()); i++ {
		e.walkTopLevel(root.Child(i))
	}

	e.popScope()

	return e.buildChains(), nil
}

func (e *JSDefUseExtractor) pushScope() {
	e.scopeStack = append(e.scopeStack, make(map[string]VarDef))
}

func (e *JSDefUseExtractor) popScope() {
	e.scopeStack = e.scopeStack[:len(e.scopeStack)-1]
}

func (e *JSDefUseExtractor) addDef(name string, line int, scope ScopeType) VarDef {
	id := fmt.Sprintf("%s.%s:%d:%s", e.funcPkg, e.funcFile, line, name)
	def := VarDef{
		ID:        id,
		Variable:  name,
		Pkg:       e.funcPkg,
		File:      e.funcFile,
		StartLine: line,
		EndLine:   line,
	}
	e.defs = append(e.defs, def)

	if len(e.scopeStack) > 0 {
		e.scopeStack[len(e.scopeStack)-1][name] = def
	}

	if _, exists := e.variables[name]; !exists {
		e.variables[name] = &Variable{
			Name:    name,
			Scope:   scope,
			Pkg:     e.funcPkg,
			File:    e.funcFile,
			DefLine: line,
		}
	}

	return def
}

func (e *JSDefUseExtractor) addUse(name string, line int, kind UseKind) {
	def := e.resolveIdent(name)
	if def == nil {
		return
	}

	useID := fmt.Sprintf("%s.%s:%d:%s:%d", e.funcPkg, e.funcFile, line, name, int(kind))
	use := VarUse{
		ID:       useID,
		Variable: name,
		File:     e.funcFile,
		Line:     line,
		Kind:     kind,
		FuncID:   e.funcID,
	}
	e.uses = append(e.uses, use)
	e.defRefs[useID] = def.ID
}

func (e *JSDefUseExtractor) resolveIdent(name string) *VarDef {
	for i := len(e.scopeStack) - 1; i >= 0; i-- {
		if def, ok := e.scopeStack[i][name]; ok {
			return &def
		}
	}
	return nil
}

func (e *JSDefUseExtractor) walkTopLevel(node *sitter.Node) {
	switch node.Type() {
	case "function_declaration", "generator_function_declaration":
		e.walkFuncDecl(node)
	case "class_declaration":
		e.walkClassDecl(node)
	case "lexical_declaration", "variable_statement":
		e.walkVarDecl(node)
	case "export_statement":
		for i := 0; i < int(node.ChildCount()); i++ {
			e.walkTopLevel(node.Child(i))
		}
	case "expression_statement":
		if node.ChildCount() > 0 {
			e.walkTopLevel(node.Child(0))
		}
	case "internal_module", "module":
		body := childOfType(node, "statement_block")
		if body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				e.walkTopLevel(body.Child(i))
			}
		}
	case "interface_declaration", "type_alias_declaration", "enum_declaration":
		nameNode := childOfType(node, "type_identifier", "identifier")
		if nameNode != nil {
			name := string(nameNode.Content(e.src))
			line := int(node.StartPoint().Row) + 1
			e.addDef(name, line, ScopeGlobal)
		}
	}
}

func (e *JSDefUseExtractor) walkFuncDecl(node *sitter.Node) {
	nameNode := childOfType(node, "identifier")
	if nameNode == nil {
		return
	}
	name := string(nameNode.Content(e.src))
	line := int(node.StartPoint().Row) + 1
	e.funcID = e.pkg + "." + name
	e.addDef(name, line, ScopeGlobal)

	e.pushScope()

	params := childOfType(node, "formal_parameters")
	if params != nil {
		for i := 0; i < int(params.ChildCount()); i++ {
			e.walkParam(params.Child(i))
		}
	}

	body := childOfType(node, "statement_block")
	if body != nil {
		e.walkBlock(body)
	}

	e.popScope()
}

func (e *JSDefUseExtractor) walkArrowFunc(node *sitter.Node) {
	line := int(node.StartPoint().Row) + 1

	e.pushScope()

	params := childOfType(node, "formal_parameters", "identifier")
	if params != nil && params.Type() == "identifier" {
		pname := string(params.Content(e.src))
		e.addDef(pname, line, ScopeParam)
	} else if params != nil {
		for i := 0; i < int(params.ChildCount()); i++ {
			e.walkParam(params.Child(i))
		}
	}

	body := childOfType(node, "statement_block")
	if body != nil {
		e.walkBlock(body)
	} else {
		expr := childOfType(node, "binary_expression", "call_expression", "identifier", "member_expression",
			"unary_expression", "ternary_expression", "template_string", "parenthesized_expression")
		if expr != nil {
			e.walkExpr(expr, UseReturn)
		}
	}

	e.popScope()
}

func (e *JSDefUseExtractor) walkClassDecl(node *sitter.Node) {
	nameNode := childOfType(node, "type_identifier", "identifier")
	if nameNode != nil {
		name := string(nameNode.Content(e.src))
		line := int(node.StartPoint().Row) + 1
		e.addDef(name, line, ScopeGlobal)
	}

	body := childOfType(node, "class_body")
	if body != nil {
		for i := 0; i < int(body.ChildCount()); i++ {
			child := body.Child(i)
			if child.Type() == "method_definition" {
				e.walkMethodDef(child)
			}
		}
	}
}

func (e *JSDefUseExtractor) walkMethodDef(node *sitter.Node) {
	nameNode := childOfType(node, "property_identifier", "identifier")
	if nameNode == nil {
		return
	}
	name := string(nameNode.Content(e.src))
	e.funcID = e.pkg + "." + name

	e.pushScope()

	params := childOfType(node, "formal_parameters")
	if params != nil {
		for i := 0; i < int(params.ChildCount()); i++ {
			e.walkParam(params.Child(i))
		}
	}

	body := childOfType(node, "statement_block")
	if body != nil {
		e.walkBlock(body)
	}

	e.popScope()
}

func (e *JSDefUseExtractor) walkParam(node *sitter.Node) {
	line := int(node.StartPoint().Row) + 1

	switch node.Type() {
	case "identifier":
		name := string(node.Content(e.src))
		e.addDef(name, line, ScopeParam)

	case "required_parameter", "optional_parameter":
		nameNode := childOfType(node, "identifier")
		if nameNode != nil {
			name := string(nameNode.Content(e.src))
			e.addDef(name, line, ScopeParam)
		}

	case "object_pattern":
		for i := 0; i < int(node.ChildCount()); i++ {
			prop := node.Child(i)
			if prop.Type() == "shorthand_property_identifier_pattern" {
				name := string(prop.Content(e.src))
				e.addDef(name, line, ScopeParam)
			} else if prop.Type() == "pair_pattern" {
				val := childOfType(prop, "identifier")
				if val != nil {
					name := string(val.Content(e.src))
					e.addDef(name, line, ScopeParam)
				}
			}
		}

	case "array_pattern":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "identifier" {
				name := string(child.Content(e.src))
				e.addDef(name, line, ScopeParam)
			}
		}
	}
}

func (e *JSDefUseExtractor) walkVarDecl(node *sitter.Node) {
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if child.Type() != "variable_declarator" {
			continue
		}
		line := int(child.StartPoint().Row) + 1

		nameNode := childOfType(child, "identifier")
		if nameNode != nil {
			name := string(nameNode.Content(e.src))
			e.addDef(name, line, ScopeLocal)
		}

		val := childOfType(child, "arrow_function", "function")
		if val != nil {
			if val.Type() == "arrow_function" {
				e.walkArrowFunc(val)
			} else {
				e.walkFuncFromVar(child)
			}
		}

		init := child.ChildByFieldName("value")
		if init != nil && init.Type() != "arrow_function" && init.Type() != "function" {
			e.walkExpr(init, UseRead)
		}
	}
}

func (e *JSDefUseExtractor) walkFuncFromVar(node *sitter.Node) {
	fn := childOfType(node, "function", "generator_function")
	if fn == nil {
		return
	}
	e.walkFuncDecl(fn)
}

func (e *JSDefUseExtractor) walkBlock(node *sitter.Node) {
	for i := 0; i < int(node.ChildCount()); i++ {
		e.walkStmt(node.Child(i))
	}
}

func (e *JSDefUseExtractor) walkStmt(node *sitter.Node) {
	switch node.Type() {
	case "lexical_declaration", "variable_statement":
		e.walkVarDecl(node)

	case "expression_statement":
		expr := node.Child(0)
		if expr != nil {
			e.walkExpr(expr, UseRead)
		}

	case "return_statement":
		val := childOfType(node, "binary_expression", "call_expression", "identifier", "member_expression",
			"unary_expression", "ternary_expression", "template_string", "object", "array",
			"await_expression", "parenthesized_expression")
		if val != nil {
			e.walkExpr(val, UseReturn)
		}

	case "if_statement":
		cond := childOfType(node, "parenthesized_expression")
		if cond != nil {
			inner := cond.Child(1)
			if inner != nil {
				e.walkExpr(inner, UseRead)
			}
		}
		cons := childOfType(node, "statement_block")
		if cons != nil {
			e.walkBlock(cons)
		}
		alt := childOfType(node, "else_clause")
		if alt != nil {
			altBlock := childOfType(alt, "statement_block")
			if altBlock != nil {
				e.walkBlock(altBlock)
			}
		}

	case "for_statement", "for_in_statement", "for_of_statement":
		left := childOfType(node, "identifier", "lexical_declaration")
		if left != nil {
			if left.Type() == "identifier" {
				name := string(left.Content(e.src))
				line := int(left.StartPoint().Row) + 1
				e.addDef(name, line, ScopeLocal)
			} else {
				e.walkVarDecl(left)
			}
		}
		body := childOfType(node, "statement_block")
		if body != nil {
			e.walkBlock(body)
		}

	case "while_statement", "do_statement":
		body := childOfType(node, "statement_block")
		if body != nil {
			e.walkBlock(body)
		}

	case "switch_statement":
		body := childOfType(node, "switch_body")
		if body != nil {
			for i := 0; i < int(body.ChildCount()); i++ {
				caseNode := body.Child(i)
				if caseNode.Type() == "switch_case" || caseNode.Type() == "default_case" {
					for j := 0; j < int(caseNode.ChildCount()); j++ {
						e.walkStmt(caseNode.Child(j))
					}
				}
			}
		}

	case "try_statement":
		body := childOfType(node, "statement_block")
		if body != nil {
			e.walkBlock(body)
		}
		catch := childOfType(node, "catch_clause")
		if catch != nil {
			param := childOfType(catch, "identifier")
			if param != nil {
				name := string(param.Content(e.src))
				line := int(param.StartPoint().Row) + 1
				e.addDef(name, line, ScopeLocal)
			}
			catchBody := childOfType(catch, "statement_block")
			if catchBody != nil {
				e.walkBlock(catchBody)
			}
		}

	case "throw_statement":
		val := childOfType(node, "identifier", "call_expression", "object", "new_expression")
		if val != nil {
			e.walkExpr(val, UseRead)
		}

	case "block":
		e.pushScope()
		e.walkBlock(node)
		e.popScope()
	}
}

func (e *JSDefUseExtractor) walkExpr(node *sitter.Node, kind UseKind) {
	if node == nil {
		return
	}
	line := int(node.StartPoint().Row) + 1

	switch node.Type() {
	case "identifier":
		e.addUse(string(node.Content(e.src)), line, kind)

	case "member_expression":
		obj := node.Child(0)
		if obj != nil && obj.Type() == "identifier" {
			e.addUse(string(obj.Content(e.src)), line, kind)
		}

	case "call_expression":
		fn := node.ChildByFieldName("function")
		if fn != nil && fn.Type() == "identifier" {
			e.addUse(string(fn.Content(e.src)), line, UseRead)
		}
		if fn != nil && fn.Type() == "member_expression" {
			obj := fn.Child(0)
			if obj != nil && obj.Type() == "identifier" {
				e.addUse(string(obj.Content(e.src)), line, UseRead)
			}
		}
		args := childOfType(node, "arguments")
		if args != nil {
			for i := 0; i < int(args.ChildCount()); i++ {
				e.walkExpr(args.Child(i), UseCallArg)
			}
		}

	case "assignment_expression":
		left := node.Child(0)
		if left != nil && left.Type() == "identifier" {
			name := string(left.Content(e.src))
			e.addUse(name, line, UseWrite)
		}
		right := node.Child(2)
		if right != nil {
			e.walkExpr(right, UseRead)
		}

	case "binary_expression", "unary_expression":
		for i := 0; i < int(node.ChildCount()); i++ {
			e.walkExpr(node.Child(i), kind)
		}

	case "parenthesized_expression":
		if node.ChildCount() >= 2 {
			e.walkExpr(node.Child(1), kind)
		}

	case "await_expression":
		val := node.Child(1)
		if val != nil {
			e.walkExpr(val, kind)
		}

	case "new_expression":
		ctor := node.Child(1)
		if ctor != nil && ctor.Type() == "identifier" {
			e.addUse(string(ctor.Content(e.src)), line, UseRead)
		}
		args := childOfType(node, "arguments")
		if args != nil {
			for i := 0; i < int(args.ChildCount()); i++ {
				e.walkExpr(args.Child(i), UseCallArg)
			}
		}

	case "arrow_function":
		e.walkArrowFunc(node)

	case "template_string":
		for i := 0; i < int(node.ChildCount()); i++ {
			child := node.Child(i)
			if child.Type() == "template_substitution" {
				inner := childOfType(child, "identifier", "binary_expression", "call_expression")
				if inner != nil {
					e.walkExpr(inner, kind)
				}
			}
		}

	case "object":
		for i := 0; i < int(node.ChildCount()); i++ {
			pair := node.Child(i)
			if pair.Type() == "pair" {
				val := pair.Child(1)
				if val != nil {
					e.walkExpr(val, kind)
				}
			}
		}

	case "array":
		for i := 0; i < int(node.ChildCount()); i++ {
			e.walkExpr(node.Child(i), kind)
		}

	case "ternary_expression":
		for i := 0; i < int(node.ChildCount()); i++ {
			e.walkExpr(node.Child(i), kind)
		}

	case "subscript_expression":
		for i := 0; i < int(node.ChildCount()); i++ {
			e.walkExpr(node.Child(i), kind)
		}

	case "spread_element":
		val := node.Child(1)
		if val != nil {
			e.walkExpr(val, kind)
		}
	}
}

func (e *JSDefUseExtractor) buildChains() []*DefUseChain {
	chainMap := make(map[string]*DefUseChain)
	for i := range e.defs {
		chainMap[e.defs[i].ID] = &DefUseChain{
			Def:  e.defs[i],
			Uses: nil,
		}
	}

	for _, use := range e.uses {
		if defID, ok := e.defRefs[use.ID]; ok {
			if chain, ok := chainMap[defID]; ok {
				chain.Uses = append(chain.Uses, use)
			}
		}
	}

	chains := make([]*DefUseChain, 0, len(chainMap))
	for _, chain := range chainMap {
		chains = append(chains, chain)
	}
	return chains
}

func childOfType(node *sitter.Node, types ...string) *sitter.Node {
	set := make(map[string]bool, len(types))
	for _, t := range types {
		set[t] = true
	}
	for i := 0; i < int(node.ChildCount()); i++ {
		child := node.Child(i)
		if set[child.Type()] {
			return child
		}
	}
	return nil
}

func jsLang(filePath string) *sitter.Language {
	switch filepath.Ext(filePath) {
	case ".ts":
		return ts.GetLanguage()
	case ".tsx":
		return tsx.GetLanguage()
	default:
		return javascript.GetLanguage()
	}
}
