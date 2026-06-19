package engine

import (
	"context"
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/storage"
)

func contextVarTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	kv, err := storage.OpenStore(dir + "/kv")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	cs := storage.NewChunkStore(kv)
	if err := cs.Put(storage.ChunkMeta{
		ID:         "main.go:1-5",
		FilePath:   "main.go",
		ChunkType:  "4",
		SymbolName: "foo",
		StartLine:  1,
		EndLine:    5,
		Content:    "func foo(x int) {\n\ty := x + 1\n\treturn y\n}",
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "foo", File: "main.go", Line: 1})

	fi := dataflow.NewFlowIndex()
	fg := &dataflow.FlowGraph{
		Variables: map[string]*dataflow.Variable{
			"x": {Name: "x", Type: "int", Scope: dataflow.ScopeParam, Pkg: "main", File: "main.go", DefLine: 1},
			"y": {Name: "y", Type: "int", Scope: dataflow.ScopeLocal, Pkg: "main", File: "main.go", DefLine: 2},
		},
		Defs: make(map[string]*dataflow.VarDef),
		Uses: make(map[string]*dataflow.VarUse),
		DefUseMap: map[string]*dataflow.DefUseChain{
			"main.main.go:2:y": {
				Def: dataflow.VarDef{ID: "main.main.go:2:y", Variable: "y", Pkg: "main", File: "main.go", StartLine: 2},
				Uses: []dataflow.VarUse{
					{ID: "main.main.go:3:y:3", Variable: "y", File: "main.go", Line: 3, Kind: dataflow.UseReturn, FuncID: "main.foo"},
				},
			},
		},
		TypeNodes: make(map[string]*dataflow.TypeFlowNode),
	}
	fi.BuildFromFlowGraph(fg)

	return &Engine{
		chunkStore: cs,
		flowIndex:  fi,
		graph:      graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph:  cg,
		tokenizer:  indexer.NewTokenizer(),
	}
}

func TestContextVariablesSection(t *testing.T) {
	eng := contextVarTestEngine(t)
	resp, err := eng.getContextPack(context.Background(), Query{Text: "main.go:1-5"}, &Response{})
	if err != nil {
		t.Fatalf("getContextPack: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
	content := resp.Results[0].Content
	if !contains(content, "variables") {
		t.Errorf("expected variables section in context pack")
	}
}

func TestContextDataFlowSection(t *testing.T) {
	eng := contextVarTestEngine(t)
	resp, err := eng.getContextPack(context.Background(), Query{Text: "main.go:1-5"}, &Response{})
	if err != nil {
		t.Fatalf("getContextPack: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
	content := resp.Results[0].Content
	if !contains(content, "dataflow") {
		t.Errorf("expected dataflow section in context pack")
	}
}

func TestContextVariablesNoSymbol(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(dir + "/kv")
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	cs := storage.NewChunkStore(kv)
	if err := cs.Put(storage.ChunkMeta{
		ID:        "main.go:1-3",
		FilePath:  "main.go",
		ChunkType: "0",
		StartLine: 1,
		EndLine:   3,
		Content:   "package main",
	}); err != nil {
		t.Fatalf("Put: %v", err)
	}

	eng := &Engine{
		chunkStore: cs,
		flowIndex:  dataflow.NewFlowIndex(),
		tokenizer:  indexer.NewTokenizer(),
	}

	// Chunk tanpa SymbolName → tidak ada variable section
	vars := eng.contextVariables(&storage.ChunkMeta{ID: "main.go:1-3", FilePath: "main.go"})
	if vars != "" {
		t.Errorf("expected empty variables for no-symbol chunk, got %q", vars)
	}
}

func TestContextBudgetAware(t *testing.T) {
	eng := contextVarTestEngine(t)
	resp, err := eng.getContextPack(context.Background(), Query{Text: "main.go:1-5", MaxTokens: 10}, &Response{})
	if err != nil {
		t.Fatalf("getContextPack: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && containsStr(s, substr)
}

func containsStr(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
