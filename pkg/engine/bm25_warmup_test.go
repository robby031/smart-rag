package engine

import (
	"context"
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/search"
	"github.com/robby031/smart-rag/pkg/storage"
)

func TestBM25WarmupOnRestart(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(dir + "/kv")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()

	cs := storage.NewChunkStore(kv)
	gs := storage.NewGraphStore(kv)

	if err := cs.PutAll([]storage.ChunkMeta{
		{
			ID:         "pkg/engine/engine.go:1-50",
			FilePath:   "pkg/engine/engine.go",
			ChunkType:  "4",
			SymbolName: "New",
			Content:    "func New() creates the engine instance with BM25 search",
		},
		{
			ID:         "pkg/engine/index.go:1-30",
			FilePath:   "pkg/engine/index.go",
			ChunkType:  "4",
			SymbolName: "IndexFile",
			Content:    "func IndexFile indexes a single file into BoltDB and BM25",
		},
	}); err != nil {
		t.Fatalf("put chunks: %v", err)
	}

	eng := &Engine{
		chunker:     indexer.NewChunker(512),
		parser:      indexer.NewParser(),
		tokenizer:   indexer.NewTokenizer(),
		bm25:        search.NewBM25(),
		astSearch:   nil,
		graph:       graph.NewGraph(graph.NewCallGraph(), graph.NewImportGraph()),
		callGraph:   graph.NewPersistentCallGraph(gs),
		importGraph: graph.NewPersistentImportGraph(gs),
		chunkStore:  cs,
	}

	if !eng.bm25.IsEmpty() {
		t.Fatal("expected BM25 to be empty before FinalizeIndex")
	}

	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	results, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "BM25 search engine",
	})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(results.Results) == 0 {
		t.Error("expected results after BM25 warmup, got empty results")
	}
}

func TestBM25NoDoubleWarmup(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(dir + "/kv")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()

	cs := storage.NewChunkStore(kv)
	gs := storage.NewGraphStore(kv)

	eng := &Engine{
		chunker:     indexer.NewChunker(512),
		parser:      indexer.NewParser(),
		tokenizer:   indexer.NewTokenizer(),
		bm25:        search.NewBM25(),
		astSearch:   nil,
		graph:       graph.NewGraph(graph.NewCallGraph(), graph.NewImportGraph()),
		callGraph:   graph.NewPersistentCallGraph(gs),
		importGraph: graph.NewPersistentImportGraph(gs),
		chunkStore:  cs,
	}

	eng.bm25.AddDocument(map[string]int{"engine": 1, "index": 1}, "chunk:a")
	eng.bm25.AddDocument(map[string]int{"search": 1, "bm25": 1}, "chunk:b")

	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	if len(eng.bm25.DocIDs) != 2 {
		t.Errorf("expected 2 docs, got %d", len(eng.bm25.DocIDs))
	}
}
