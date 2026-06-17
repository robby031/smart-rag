package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/storage"
)

func TestFinalizeIndexRefreshesChunkReachability(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package main",
		"",
		"func main() { live() }",
		"",
		"func live() { helper() }",
		"",
		"func helper() {}",
		"",
		"func dead() {}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "cmd/app/main.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	live := mustChunkBySymbol(t, eng.chunkStore, "live")
	if live.Reachability != ReachabilityReachable {
		t.Fatalf("live reachability mismatch: %+v", live)
	}
	if live.ContextWeight != reachableContextWeight {
		t.Fatalf("live context weight mismatch: %.4f", live.ContextWeight)
	}

	dead := mustChunkBySymbol(t, eng.chunkStore, "dead")
	if dead.Reachability != ReachabilityUnreachable {
		t.Fatalf("dead reachability mismatch: %+v", dead)
	}
	if dead.ContextWeight != unreachableContextWeight {
		t.Fatalf("dead context weight mismatch: %.4f", dead.ContextWeight)
	}
}

func TestSearchAppliesReachabilityPenaltyWithoutDroppingResults(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/live.go:1-3",
			FilePath:      "pkg/live.go",
			ChunkType:     "4",
			SymbolName:    "Live",
			StartLine:     1,
			EndLine:       3,
			Content:       "func Live() { sharedPruningToken() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:            "pkg/dead.go:1-3",
			FilePath:      "pkg/dead.go",
			ChunkType:     "4",
			SymbolName:    "dead",
			StartLine:     1,
			EndLine:       3,
			Content:       "func dead() { sharedPruningToken() }",
			Reachability:  ReachabilityUnreachable,
			ContextWeight: unreachableContextWeight,
		},
		{
			ID:         "pkg/other.go:1-3",
			FilePath:   "pkg/other.go",
			ChunkType:  "4",
			SymbolName: "Other",
			StartLine:  1,
			EndLine:    3,
			Content:    "func Other() { unrelatedToken() }",
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "sharedPruningToken",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected both reachable and unreachable results, got %d", len(resp.Results))
	}
	if got, want := resp.Results[0].Chunk.ID, "pkg/live.go:1-3"; got != want {
		t.Fatalf("reachable chunk should rank first: want %s, got %s", want, got)
	}
	if got, want := resp.Results[1].Chunk.ID, "pkg/dead.go:1-3"; got != want {
		t.Fatalf("unreachable chunk should still be returned: want %s, got %s", want, got)
	}
	if !strings.Contains(strings.Join(resp.Results[1].Related, "\n"), "penalty reachability=unreachable") {
		t.Fatalf("expected reachability penalty explanation, got %v", resp.Results[1].Related)
	}
}

func TestContextPackSkipsUnreachableNearbyChunks(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/service.go:1-3",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "deadNearby",
			StartLine:     1,
			EndLine:       3,
			Content:       "func deadNearby() { lowValueBoilerplate() }",
			Reachability:  ReachabilityUnreachable,
			ContextWeight: unreachableContextWeight,
		},
		{
			ID:            "pkg/service.go:5-8",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "Target",
			StartLine:     5,
			EndLine:       8,
			Content:       "func Target() { importantLogic() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:            "pkg/service.go:10-12",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "liveNearby",
			StartLine:     10,
			EndLine:       12,
			Content:       "func liveNearby() { usefulFollowup() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QueryContextPack,
		Text: "pkg/service.go:5-8",
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}
	content := resp.Results[0].Content
	if strings.Contains(content, "deadNearby") {
		t.Fatalf("unreachable nearby chunk leaked into context pack:\n%s", content)
	}
	if !strings.Contains(content, "liveNearby") {
		t.Fatalf("reachable nearby chunk missing from context pack:\n%s", content)
	}
}

func pruningTestEngine(t *testing.T) (*Engine, func()) {
	t.Helper()

	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return New(kv, storage.NewChunkStore(kv), nil, nil), func() {
		if err := kv.Close(); err != nil {
			t.Fatalf("Close store: %v", err)
		}
	}
}

func mustChunkBySymbol(t *testing.T, store *storage.ChunkStore, symbol string) *storage.ChunkMeta {
	t.Helper()

	chunks, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	for _, chunk := range chunks {
		if chunk.SymbolName == symbol {
			return chunk
		}
	}
	t.Fatalf("chunk with symbol %q not found", symbol)
	return nil
}
