package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/storage"
)

func TestReadSnippetOrdersChunksByLineRange(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	cs := storage.NewChunkStore(kv)
	filePath := "pkg/example/file.go"
	if err := cs.PutAll([]storage.ChunkMeta{
		{
			ID:        filePath + ":1-1",
			FilePath:  filePath,
			StartLine: 1,
			EndLine:   1,
			Content:   "line 1",
		},
		{
			ID:        filePath + ":10-10",
			FilePath:  filePath,
			StartLine: 10,
			EndLine:   10,
			Content:   "line 10",
		},
		{
			ID:        filePath + ":2-2",
			FilePath:  filePath,
			StartLine: 2,
			EndLine:   2,
			Content:   "line 2",
		},
	}); err != nil {
		t.Fatalf("PutAll: %v", err)
	}

	eng := &Engine{chunkStore: cs}
	resp, err := eng.Query(context.Background(), Query{
		Type: QueryReadSnippet,
		Text: filePath + ":1-10",
	})
	if err != nil {
		t.Fatalf("readSnippet: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if got, want := resp.Results[0].Content, "line 1\nline 2\nline 10"; got != want {
		t.Fatalf("snippet content mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestContextPackAssemblesPrimaryNearbyAndRelated(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:         "pkg/service/service.go:1-5",
			FilePath:   "pkg/service/service.go",
			SymbolName: "previousChunk",
			StartLine:  1,
			EndLine:    5,
			Content:    "func previousChunk() { prepareDependency() }",
		},
		{
			ID:         "pkg/service/service.go:6-12",
			FilePath:   "pkg/service/service.go",
			SymbolName: "TargetSymbol",
			StartLine:  6,
			EndLine:    12,
			Content:    "func TargetSymbol() { callDatabase() }",
		},
		{
			ID:         "pkg/service/service.go:13-20",
			FilePath:   "pkg/service/service.go",
			SymbolName: "nextChunk",
			StartLine:  13,
			EndLine:    20,
			Content:    "func nextChunk() { cleanupDependency() }",
		},
	})

	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "svc", Name: "TargetSymbol", File: "pkg/service/service.go", Line: 6})
	cg.AddNode(&graph.Node{Pkg: "svc", Name: "Caller", File: "pkg/service/caller.go", Line: 4})
	cg.AddNode(&graph.Node{Pkg: "svc", Name: "Callee", File: "pkg/service/callee.go", Line: 9})
	cg.AddEdge("svc.Caller", "svc.TargetSymbol", 4, "pkg/service/caller.go")
	cg.AddEdge("svc.TargetSymbol", "svc.Callee", 8, "pkg/service/service.go")
	cg.BuildInEdges()
	eng.graph = graph.NewGraph(cg, graph.NewImportGraph())

	resp, err := eng.Query(context.Background(), Query{
		Type: QueryContextPack,
		Text: "pkg/service/service.go:6-12",
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one context pack result, got %d", len(resp.Results))
	}

	content := resp.Results[0].Content
	for _, want := range []string{
		"## primary",
		"Symbol: TargetSymbol",
		"## nearby",
		"previousChunk",
		"nextChunk",
		"## related",
		"definition: svc.TargetSymbol",
		"caller: svc.Caller",
		"callee: svc.Callee",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("expected context pack to contain %q:\n%s", want, content)
		}
	}
}

func TestContextPackBudgetKeepsPrimaryFirst(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:         "pkg/service/service.go:1-5",
			FilePath:   "pkg/service/service.go",
			SymbolName: "previousChunk",
			StartLine:  1,
			EndLine:    5,
			Content:    "func previousChunk() { prepareDependency() }",
		},
		{
			ID:         "pkg/service/service.go:6-12",
			FilePath:   "pkg/service/service.go",
			SymbolName: "TargetSymbol",
			StartLine:  6,
			EndLine:    12,
			Content:    strings.Repeat("func TargetSymbol() { callDatabase() }\n", 20),
		},
		{
			ID:         "pkg/service/service.go:13-20",
			FilePath:   "pkg/service/service.go",
			SymbolName: "nextChunk",
			StartLine:  13,
			EndLine:    20,
			Content:    "func nextChunk() { cleanupDependency() }",
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type:      QueryContextPack,
		Text:      "pkg/service/service.go:6-12",
		MaxTokens: 220,
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}

	content := resp.Results[0].Content
	if len(content) > 220 {
		t.Fatalf("context pack exceeded budget: %d", len(content))
	}
	if !strings.HasPrefix(content, "## primary") {
		t.Fatalf("primary section must remain first under budget pressure:\n%s", content)
	}
	if strings.Contains(content, "## nearby") {
		t.Fatalf("nearby section should be dropped before truncating priority order:\n%s", content)
	}
	if !strings.Contains(content, "...[truncated]") {
		t.Fatalf("expected primary truncation marker under tight budget:\n%s", content)
	}
}
