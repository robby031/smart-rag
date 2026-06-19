package indexer_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/indexer"
)

func TestExtractFuncVar(t *testing.T) {
	src := `package main

func foo() {
	x := 42
}`
	ve := indexer.NewVariableExtractor()
	decl := indexer.ParsedDecl{
		Name:      "foo",
		Kind:      indexer.DeclFunc,
		StartLine: 3,
		EndLine:   5,
		Content:   "func foo() {\n\tx := 42\n}",
	}

	refs := ve.ExtractVariables(decl, src, "main")
	if len(refs) == 0 {
		t.Fatal("expected at least 1 variable ref")
	}

	found := false
	for _, r := range refs {
		if r.Name == "x" && r.IsDef {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected def for x")
	}
}

func TestExtractParam(t *testing.T) {
	src := `package main

func bar(name string, age int) {
	_ = name
	_ = age
}`
	ve := indexer.NewVariableExtractor()
	decl := indexer.ParsedDecl{
		Name:      "bar",
		Kind:      indexer.DeclFunc,
		StartLine: 3,
		EndLine:   6,
		Content:   "func bar(name string, age int) {\n\t_ = name\n\t_ = age\n}",
	}

	refs := ve.ExtractVariables(decl, src, "main")
	if len(refs) == 0 {
		t.Fatal("expected variable refs")
	}

	hasName := false
	hasAge := false
	for _, r := range refs {
		if r.Name == "name" && r.IsDef {
			hasName = true
		}
		if r.Name == "age" && r.IsDef {
			hasAge = true
		}
	}
	if !hasName {
		t.Errorf("expected def for name")
	}
	if !hasAge {
		t.Errorf("expected def for age")
	}
}

func TestExtractNoVariables(t *testing.T) {
	src := `package main

type Config struct {
	Host string
}`
	ve := indexer.NewVariableExtractor()
	decl := indexer.ParsedDecl{
		Name:      "Config",
		Kind:      indexer.DeclStruct,
		StartLine: 3,
		EndLine:   5,
		Content:   "type Config struct {\n\tHost string\n}",
	}

	refs := ve.ExtractVariables(decl, src, "main")
	if len(refs) != 0 {
		t.Errorf("expected 0 refs for struct decl, got %d", len(refs))
	}
}

func TestExtractEmptyDecl(t *testing.T) {
	ve := indexer.NewVariableExtractor()
	refs := ve.ExtractVariables(indexer.ParsedDecl{}, "", "main")
	if refs != nil {
		t.Errorf("expected nil for empty decl")
	}
}

func TestChunkerBackwardCompat(t *testing.T) {
	chunker := indexer.NewChunker(512)
	decls := []indexer.ParsedDecl{
		{
			Name:      "foo",
			Kind:      indexer.DeclFunc,
			StartLine: 1,
			EndLine:   3,
			Content:   "func foo() {}",
		},
	}
	meta := indexer.FileMeta{Package: "main"}
	chunks := chunker.Chunk(decls, "main.go", meta)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
}

func TestChunkerWithVars(t *testing.T) {
	src := `package main

func foo() {
	x := 42
	_ = x
}`
	chunker := indexer.NewChunker(512)
	decls := []indexer.ParsedDecl{
		{
			Name:      "foo",
			Kind:      indexer.DeclFunc,
			StartLine: 3,
			EndLine:   6,
			Content:   "func foo() {\n\tx := 42\n\t_ = x\n}",
		},
	}
	meta := indexer.FileMeta{Package: "main"}
	ve := indexer.NewVariableExtractor()

	chunks := chunker.ChunkWithVars(decls, "main.go", meta, ve, src)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}

	hasVars := false
	for _, ch := range chunks {
		if len(ch.Variables) > 0 {
			hasVars = true
			break
		}
	}
	if !hasVars {
		t.Errorf("expected chunks with variables")
	}
}

func TestChunkerWithVarsNilExtractor(t *testing.T) {
	chunker := indexer.NewChunker(512)
	decls := []indexer.ParsedDecl{
		{
			Name:      "foo",
			Kind:      indexer.DeclFunc,
			StartLine: 1,
			EndLine:   3,
			Content:   "func foo() {}",
		},
	}
	meta := indexer.FileMeta{Package: "main"}

	chunks := chunker.ChunkWithVars(decls, "main.go", meta, nil, "")
	if len(chunks) == 0 {
		t.Fatal("expected chunks with nil extractor")
	}
}
