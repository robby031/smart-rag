package engine

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/storage"
)

func definitionTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	cs := storage.NewChunkStore(kv)

	if err := cs.PutAll([]storage.ChunkMeta{
		{
			ID:         "pkg/engine/engine.go:14-24",
			FilePath:   "pkg/engine/engine.go",
			ChunkType:  fmt.Sprintf("%d", indexer.ChunkStruct),
			SymbolName: "Engine",
			StartLine:  14,
			EndLine:    24,
			Content:    "type Engine struct { chunker *indexer.Chunker }",
		},
		{
			ID:         "pkg/engine/engine.go:26-45",
			FilePath:   "pkg/engine/engine.go",
			ChunkType:  fmt.Sprintf("%d", indexer.ChunkFunc),
			SymbolName: "New",
			StartLine:  26,
			EndLine:    45,
			Content:    "func New(...) *Engine { ... }",
		},
	}); err != nil {
		t.Fatalf("PutAll: %v", err)
	}

	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "engine", Name: "IndexFile", Recv: "*Engine", File: "pkg/engine/index.go", Line: 17})
	cg.AddNode(&graph.Node{Pkg: "engine", Name: "IndexDir", Recv: "*Engine", File: "pkg/engine/index.go", Line: 77})
	cg.AddNode(&graph.Node{Pkg: "engine", Name: "New", File: "pkg/engine/engine.go", Line: 26})

	eng := &Engine{
		chunkStore: cs,
		graph:      graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph:  cg,
	}
	return eng
}

func TestFindDefinitionTypeFirst(t *testing.T) {
	eng := definitionTestEngine(t)

	resp, err := eng.findDefinition(context.Background(), Query{Text: "Engine"}, &Response{})
	if err != nil {
		t.Fatalf("findDefinition: %v", err)
	}

	if len(resp.Results) == 0 {
		t.Fatal("no results returned")
	}

	first := resp.Results[0].Content
	if !strings.HasPrefix(first, "[struct]") {
		t.Errorf("expected first result to be [struct], got: %q", first)
	}
	if !strings.Contains(first, "Engine") {
		t.Errorf("first result must contain symbol name 'Engine', got: %q", first)
	}

	var foundMethod bool
	for _, r := range resp.Results {
		if strings.Contains(r.Content, "IndexFile") {
			if !strings.HasPrefix(r.Content, "[method]") {
				t.Errorf("IndexFile result should be labelled [method], got: %q", r.Content)
			}
			foundMethod = true
		}
	}
	if !foundMethod {
		t.Errorf("expected IndexFile method in results, got: %v", resp.Results)
	}
}

func TestFindDefinitionLabels(t *testing.T) {
	eng := definitionTestEngine(t)

	resp, err := eng.findDefinition(context.Background(), Query{Text: "New"}, &Response{})
	if err != nil {
		t.Fatalf("findDefinition: %v", err)
	}

	for _, r := range resp.Results {
		if strings.Contains(r.Content, "engine.New") {
			if !strings.HasPrefix(r.Content, "[func]") {
				t.Errorf("top-level function should be labelled [func], got: %q", r.Content)
			}
		}
	}
}

func TestFindDefinitionNoResultsMessage(t *testing.T) {
	eng := definitionTestEngine(t)

	resp, err := eng.findDefinition(context.Background(), Query{Text: "CompletelyUnknownSymbolXYZ"}, &Response{})
	if err != nil {
		t.Fatalf("findDefinition: %v", err)
	}
	if len(resp.Results) != 1 || resp.Results[0].Content != "no definition found" {
		t.Errorf("expected 'no definition found' message, got: %v", resp.Results)
	}
}
