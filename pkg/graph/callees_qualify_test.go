package graph_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
)

func TestCalleesQualifiedNames(t *testing.T) {
	cg := graph.NewCallGraph()
	src := `package engine

func (e *Engine) indexFileWith() {
	chunks    := e.chunker.Chunk()
	tokens    := e.tokenizer.Tokenize(chunks)
	_          = e.bm25.AddDocument(tokens)
	_          = e.callGraph.ParseAST(nil, nil, "", "")
	_          = e.importGraph.AddAST("pkg", nil)
	_          = e.chunkStore.PutAll(nil)
	fmt.Sprintf("%v", chunks)
}
`
	if err := cg.ParseFile("pkg/engine/index.go", src, "engine"); err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	callerID := "engine.(*Engine).indexFileWith"
	callees := cg.Callees(callerID)
	if len(callees) == 0 {
		t.Fatalf("no callees found for %s", callerID)
	}

	calleesSet := make(map[string]bool, len(callees))
	for _, c := range callees {
		calleesSet[c] = true
	}

	qualified := []string{
		"e.chunker.Chunk",
		"e.tokenizer.Tokenize",
		"e.bm25.AddDocument",
		"e.callGraph.ParseAST",
		"e.importGraph.AddAST",
		"e.chunkStore.PutAll",
	}
	for _, want := range qualified {
		if !calleesSet[want] {
			t.Errorf("expected qualified callee %q, got callees: %v", want, callees)
		}
	}

	unqualified := []string{"Chunk", "Tokenize", "AddDocument", "ParseAST", "AddAST", "PutAll"}
	for _, bad := range unqualified {
		if calleesSet[bad] {
			t.Errorf("unqualified callee %q must not appear in callees: %v", bad, callees)
		}
	}
}

func TestCalleesSimpleAndPackageCallsUnchanged(t *testing.T) {
	cg := graph.NewCallGraph()
	src := `package mypkg

func Caller() {
	helper()
	fmt.Println("x")
	os.Exit(1)
}
`
	if err := cg.ParseFile("pkg/mypkg/a.go", src, "mypkg"); err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	callees := cg.Callees("mypkg.Caller")
	set := make(map[string]bool)
	for _, c := range callees {
		set[c] = true
	}

	if !set["helper"] {
		t.Errorf("bare function call 'helper()' should appear as 'helper', got: %v", callees)
	}
	if !set["fmt.Println"] {
		t.Errorf("package call 'fmt.Println' should appear as 'fmt.Println', got: %v", callees)
	}
	if !set["os.Exit"] {
		t.Errorf("package call 'os.Exit' should appear as 'os.Exit', got: %v", callees)
	}
}
