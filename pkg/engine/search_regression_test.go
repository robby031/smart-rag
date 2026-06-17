package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/search"
	"github.com/robby031/smart-rag/pkg/storage"
)

func regressionTestEngine(t *testing.T) *Engine {
	t.Helper()

	return regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:         "pkg/engine/context.go:9-22",
			FilePath:   "pkg/engine/context.go",
			ChunkType:  "4",
			SymbolName: "getContextPack",
			StartLine:  9,
			EndLine:    22,
			Content: strings.Join([]string{
				"func (e *Engine) getContextPack(ctx context.Context, q Query, resp *Response) (*Response, error) {",
				"	chunk, err := e.chunkStore.Get(q.Text)",
				"	if err != nil {",
				"		return nil, err",
				"	}",
				"	resp.Results = append(resp.Results, Result{Chunk: chunk, Content: chunk.Content})",
				"	return resp, nil",
				"}",
			}, "\n"),
		},
		{
			ID:         "pkg/engine/search.go:8-30",
			FilePath:   "pkg/engine/search.go",
			ChunkType:  "4",
			SymbolName: "search",
			StartLine:  8,
			EndLine:    30,
			Content:    "func (e *Engine) search(ctx context.Context, q Query, resp *Response) (*Response, error) { tokens := e.tokenizer.Tokenize(q.Text); return resp, nil }",
		},
		{
			ID:         "pkg/mcp/server.go:60-76",
			FilePath:   "pkg/mcp/server.go",
			ChunkType:  "4",
			SymbolName: "handleGetContextPack",
			StartLine:  60,
			EndLine:    76,
			Content:    "func (s *SmartRAGServer) handleGetContextPack(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) { chunkID := req.Params.Arguments[\"chunk_id\"]; return mcp.NewToolResultText(\"ok\"), nil }",
		},
		{
			ID:         "docs/setup.md:1-8",
			FilePath:   "docs/setup.md",
			ChunkType:  "0",
			SymbolName: "",
			StartLine:  1,
			EndLine:    8,
			Content:    "Smart RAG setup guide for docker install and MCP client configuration.",
		},
	})
}

func regressionTestEngineWithChunks(t *testing.T, chunks []storage.ChunkMeta) *Engine {
	t.Helper()

	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	chunkStore := storage.NewChunkStore(kv)
	tokenizer := indexer.NewTokenizer()
	bm25 := search.NewBM25()

	if err := chunkStore.PutAll(chunks); err != nil {
		t.Fatalf("PutAll: %v", err)
	}
	for _, ch := range chunks {
		bm25.AddDocument(tokenizer.Tokenize(ch.Content), ch.ID)
	}
	bm25.Build()

	return &Engine{
		tokenizer:  tokenizer,
		bm25:       bm25,
		chunkStore: chunkStore,
	}
}

func TestSearchRegressionExactSymbolRanksExpectedChunkFirst(t *testing.T) {
	eng := regressionTestEngine(t)

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "getContextPack chunkStore",
		TopK: 3,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected search results")
	}

	got := resp.Results[0].Chunk.ID
	want := "pkg/engine/context.go:9-22"
	if got != want {
		t.Fatalf("top result mismatch: want %s, got %s", want, got)
	}
	if resp.Results[0].Score <= 0 {
		t.Fatalf("expected positive score for top result, got %.4f", resp.Results[0].Score)
	}
	if len(resp.Results[0].Related) == 0 || !strings.Contains(resp.Results[0].Related[0], "score bm25=") {
		t.Fatalf("expected explainable score details, got %v", resp.Results[0].Related)
	}
}

func TestSearchRegressionLanguageAndPathFilters(t *testing.T) {
	eng := regressionTestEngine(t)

	resp, err := eng.Query(context.Background(), Query{
		Type:     QuerySearch,
		Text:     "context pack chunk",
		TopK:     5,
		Language: "go",
		File:     "pkg/engine",
	})
	if err != nil {
		t.Fatalf("Query search with filters: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected filtered search results")
	}
	for _, result := range resp.Results {
		if !strings.HasSuffix(result.Chunk.FilePath, ".go") {
			t.Fatalf("language filter leaked non-Go result: %s", result.Chunk.FilePath)
		}
		if !strings.Contains(result.Chunk.FilePath, "pkg/engine") {
			t.Fatalf("path filter leaked result outside pkg/engine: %s", result.Chunk.FilePath)
		}
	}
}

func TestSearchRegressionExactSymbolBoost(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:         "pkg/usecase/usage.go:1-8",
			FilePath:   "pkg/usecase/usage.go",
			SymbolName: "UseTargetSymbol",
			StartLine:  1,
			EndLine:    8,
			Content:    "func UseTargetSymbol() { TargetSymbol(); TargetSymbol() }",
		},
		{
			ID:         "pkg/domain/target.go:1-5",
			FilePath:   "pkg/domain/target.go",
			SymbolName: "TargetSymbol",
			StartLine:  1,
			EndLine:    5,
			Content:    "func TargetSymbol() { return }",
		},
		{
			ID:         "pkg/other/unrelated.go:1-5",
			FilePath:   "pkg/other/unrelated.go",
			SymbolName: "Unrelated",
			StartLine:  1,
			EndLine:    5,
			Content:    "func Unrelated() { return }",
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "TargetSymbol",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected search results")
	}
	if got, want := resp.Results[0].Chunk.ID, "pkg/domain/target.go:1-5"; got != want {
		t.Fatalf("exact symbol boost did not rank definition first: want %s, got %s", want, got)
	}
	if !strings.Contains(strings.Join(resp.Results[0].Related, "\n"), "boost exact_symbol=") {
		t.Fatalf("expected exact symbol boost explanation, got %v", resp.Results[0].Related)
	}
}

func TestSearchRegressionDeterministicTieBreakers(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:         "pkg/beta/file.go:1-5",
			FilePath:   "pkg/beta/file.go",
			SymbolName: "AlphaSymbol",
			StartLine:  1,
			EndLine:    5,
			Content:    "func SharedRankingToken() { stable tie breaker token }",
		},
		{
			ID:         "pkg/alpha/file.go:20-25",
			FilePath:   "pkg/alpha/file.go",
			SymbolName: "BetaSymbol",
			StartLine:  20,
			EndLine:    25,
			Content:    "func SharedRankingToken() { stable tie breaker token }",
		},
		{
			ID:         "pkg/alpha/file.go:10-15",
			FilePath:   "pkg/alpha/file.go",
			SymbolName: "AlphaSymbol",
			StartLine:  10,
			EndLine:    15,
			Content:    "func SharedRankingToken() { stable tie breaker token }",
		},
		{
			ID:         "pkg/other/first.go:1-5",
			FilePath:   "pkg/other/first.go",
			SymbolName: "OtherFirst",
			StartLine:  1,
			EndLine:    5,
			Content:    "func OtherFirst() { unrelated content }",
		},
		{
			ID:         "pkg/other/second.go:1-5",
			FilePath:   "pkg/other/second.go",
			SymbolName: "OtherSecond",
			StartLine:  1,
			EndLine:    5,
			Content:    "func OtherSecond() { unrelated content }",
		},
	})

	var previous []string
	for i := 0; i < 5; i++ {
		resp, err := eng.Query(context.Background(), Query{
			Type: QuerySearch,
			Text: "shared ranking token",
			TopK: 3,
		})
		if err != nil {
			t.Fatalf("Query search: %v", err)
		}
		if len(resp.Results) != 3 {
			t.Fatalf("expected 3 tied results, got %d: %v", len(resp.Results), resp.Results)
		}
		got := []string{
			resp.Results[0].Chunk.ID,
			resp.Results[1].Chunk.ID,
			resp.Results[2].Chunk.ID,
		}
		want := []string{
			"pkg/alpha/file.go:10-15",
			"pkg/alpha/file.go:20-25",
			"pkg/beta/file.go:1-5",
		}
		if strings.Join(got, "|") != strings.Join(want, "|") {
			t.Fatalf("tie-break order mismatch: want %v, got %v", want, got)
		}
		if previous != nil && strings.Join(got, "|") != strings.Join(previous, "|") {
			t.Fatalf("search order changed between runs: previous %v, got %v", previous, got)
		}
		previous = got
	}
}

func TestSearchRegressionNoResults(t *testing.T) {
	eng := regressionTestEngine(t)

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "zzzzqqqq yyyyxxxx",
		TopK: 5,
	})
	if err != nil {
		t.Fatalf("Query search no results: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("expected no results, got %d", len(resp.Results))
	}
}

func TestContextPackRegressionReturnsRequestedChunk(t *testing.T) {
	eng := regressionTestEngine(t)

	resp, err := eng.Query(context.Background(), Query{
		Type:      QueryContextPack,
		Text:      "pkg/engine/context.go:9-22",
		MaxTokens: 80,
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one context result, got %d", len(resp.Results))
	}
	if got := resp.Results[0].Chunk.ID; got != "pkg/engine/context.go:9-22" {
		t.Fatalf("context chunk mismatch: %s", got)
	}
	if len(resp.Results[0].Content) > 80 {
		t.Fatalf("context content exceeded max token budget: %d", len(resp.Results[0].Content))
	}
	if !strings.HasPrefix(resp.Results[0].Content, "func (e *Engine) getContextPack") {
		t.Fatalf("unexpected context content: %q", resp.Results[0].Content)
	}
}

func TestReadSnippetRegressionRangeAcrossChunks(t *testing.T) {
	eng := regressionTestEngine(t)

	resp, err := eng.Query(context.Background(), Query{
		Type: QueryReadSnippet,
		Text: "pkg/engine/context.go:10-12",
	})
	if err != nil {
		t.Fatalf("Query read snippet: %v", err)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected one snippet result, got %d", len(resp.Results))
	}
	if !strings.Contains(resp.Results[0].Content, "chunkStore.Get") {
		t.Fatalf("expected snippet to include chunkStore.Get, got %q", resp.Results[0].Content)
	}
}
