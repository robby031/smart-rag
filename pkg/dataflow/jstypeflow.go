package dataflow

import (
	"context"
	"path/filepath"

	sitter "github.com/smacker/go-tree-sitter"
	ts "github.com/smacker/go-tree-sitter/typescript/typescript"
)

type JSTypeFlowTracker struct {
	nodes map[string]*TypeFlowNode
}

func NewJSTypeFlowTracker() *JSTypeFlowTracker {
	return &JSTypeFlowTracker{
		nodes: make(map[string]*TypeFlowNode),
	}
}

func (t *JSTypeFlowTracker) ExtractTypes(src []byte, filePath, pkg string) error {
	lang := jsTypeLang(filePath)
	if lang == nil {
		return nil
	}
	parser := sitter.NewParser()
	parser.SetLanguage(lang)

	tree, err := parser.ParseCtx(context.Background(), nil, src)
	if err != nil {
		return err
	}

	if t.nodes == nil {
		t.nodes = make(map[string]*TypeFlowNode)
	}

	root := tree.RootNode()
	t.walkNode(root, src, pkg, filePath)
	return nil
}

func (t *JSTypeFlowTracker) walkNode(node *sitter.Node, src []byte, pkg, filePath string) {
	switch node.Type() {
	case "interface_declaration":
		nameNode := childOfType(node, "type_identifier")
		if nameNode != nil {
			name := string(nameNode.Content(src))
			line := int(node.StartPoint().Row) + 1
			tn := t.getOrCreate(name)
			tn.DefFile = filepath.Base(filePath)
			tn.DefLine = line
			t.walkTypeBody(node, src, name)
		}

	case "type_alias_declaration":
		nameNode := childOfType(node, "type_identifier")
		if nameNode != nil {
			name := string(nameNode.Content(src))
			line := int(node.StartPoint().Row) + 1
			tn := t.getOrCreate(name)
			tn.DefFile = filepath.Base(filePath)
			tn.DefLine = line
		}

	case "function_declaration", "method_definition":
		t.walkFuncType(node, src, pkg)

	case "class_declaration":
		nameNode := childOfType(node, "type_identifier", "identifier")
		if nameNode != nil {
			name := string(nameNode.Content(src))
			line := int(node.StartPoint().Row) + 1
			tn := t.getOrCreate(name)
			tn.DefFile = filepath.Base(filePath)
			tn.DefLine = line

			implements := childOfType(node, "implements_clause")
			if implements != nil {
				for i := 0; i < int(implements.ChildCount()); i++ {
					tname := childOfType(implements.Child(i), "type_identifier")
					if tname != nil {
						pt := t.getOrCreate(string(tname.Content(src)))
						pt.UsedAsField = append(pt.UsedAsField, name)
					}
				}
			}
		}

	case "lexical_declaration", "variable_statement":
		t.walkVarType(node, src, pkg)
	}

	for i := 0; i < int(node.ChildCount()); i++ {
		t.walkNode(node.Child(i), src, pkg, filePath)
	}
}

func (t *JSTypeFlowTracker) walkFuncType(node *sitter.Node, src []byte, pkg string) {
	funcID := ""

	nameNode := childOfType(node, "identifier", "property_identifier")
	if nameNode != nil {
		funcID = pkg + "." + string(nameNode.Content(src))
	}

	params := childOfType(node, "formal_parameters")
	if params != nil {
		for i := 0; i < int(params.ChildCount()); i++ {
			t.walkParamType(params.Child(i), src, funcID)
		}
	}

	type_ := childOfType(node, "type_annotation")
	if type_ != nil {
		typeName := t.extractTypeName(type_, src)
		if typeName != "" && funcID != "" {
			nt := t.getOrCreate(typeName)
			nt.UsedAsReturn = append(nt.UsedAsReturn, funcID)
		}
	}
}

func (t *JSTypeFlowTracker) walkParamType(node *sitter.Node, src []byte, funcID string) {
	typeAnn := childOfType(node, "type_annotation")
	if typeAnn != nil {
		typeName := t.extractTypeName(typeAnn, src)
		if typeName != "" && funcID != "" {
			nt := t.getOrCreate(typeName)
			nt.UsedAsParam = append(nt.UsedAsParam, funcID)
		}
	}
}

func (t *JSTypeFlowTracker) walkVarType(node *sitter.Node, src []byte, _ string) {
	for i := 0; i < int(node.ChildCount()); i++ {
		decl := node.Child(i)
		if decl.Type() != "variable_declarator" {
			continue
		}
		typeAnn := childOfType(decl, "type_annotation")
		if typeAnn != nil {
			typeName := t.extractTypeName(typeAnn, src)
			if typeName != "" {
				t.getOrCreate(typeName)
			}
		}
	}
}

func (t *JSTypeFlowTracker) walkTypeBody(node *sitter.Node, src []byte, parentType string) {
	body := childOfType(node, "object_type")
	if body == nil {
		return
	}
	for i := 0; i < int(body.ChildCount()); i++ {
		member := body.Child(i)
		if member.Type() == "property_signature" {
			typeAnn := childOfType(member, "type_annotation")
			if typeAnn != nil {
				typeName := t.extractTypeName(typeAnn, src)
				if typeName != "" {
					pt := t.getOrCreate(typeName)
					pt.UsedAsField = append(pt.UsedAsField, parentType)
				}
			}
		}
	}
}

func (t *JSTypeFlowTracker) extractTypeName(node *sitter.Node, src []byte) string {
	inner := node.Child(1)
	if inner == nil {
		return ""
	}
	switch inner.Type() {
	case "type_identifier":
		return string(inner.Content(src))
	case "predefined_type":
		return string(inner.Content(src))
	case "generic_type":
		nameNode := childOfType(inner, "type_identifier")
		if nameNode != nil {
			return string(nameNode.Content(src))
		}
	case "array_type":
		return "[]"
	case "union_type", "intersection_type":
		return string(inner.Content(src))
	case "object_type":
		return "object"
	}
	return ""
}

func (t *JSTypeFlowTracker) getOrCreate(typeName string) *TypeFlowNode {
	if n, ok := t.nodes[typeName]; ok {
		return n
	}
	n := &TypeFlowNode{TypeName: typeName}
	t.nodes[typeName] = n
	return n
}

func (t *JSTypeFlowTracker) BuildTypeGraph() []DataFlowEdge {
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

func (t *JSTypeFlowTracker) GetAllNodes() map[string]*TypeFlowNode {
	return t.nodes
}

func jsTypeLang(filePath string) *sitter.Language {
	switch filepath.Ext(filePath) {
	case ".ts", ".tsx":
		return ts.GetLanguage()
	default:
		return nil
	}
}
