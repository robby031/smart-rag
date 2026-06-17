package engine

import (
	"strings"
	"testing"
	"time"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/search"
)

func TestStatusReportsRuntimeIndexGraphAndBM25(t *testing.T) {
	bm25 := search.NewBM25()
	bm25.AddDocument(map[string]int{"status": 1}, "chunk:status")
	bm25.Build()

	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "main", File: "main.go", Line: 1})
	cg.AddNode(&graph.Node{Pkg: "engine", Name: "Status", File: "pkg/engine/status.go", Line: 30})
	cg.AddEdge("main.main", "engine.Status", 1, "main.go")

	eng := &Engine{
		bm25:      bm25,
		callGraph: cg,
	}
	eng.SetRuntimeInfo(RuntimeInfo{
		Version: "0.3.2",
		RepoDir: "/repo",
		DBDir:   "/db",
	})
	eng.RecordIndexSummary(IndexSummary{
		Mode:      "incremental",
		Indexed:   2,
		Deleted:   1,
		UpdatedAt: time.Date(2026, 6, 18, 10, 11, 12, 0, time.UTC),
	})

	status := eng.Status()
	if status.Version != "0.3.2" {
		t.Fatalf("version mismatch: %s", status.Version)
	}
	if status.RepoDir != "/repo" || status.DBDir != "/db" {
		t.Fatalf("runtime paths mismatch: %+v", status)
	}
	if status.IndexedChunks != 1 {
		t.Fatalf("indexed chunks mismatch: %d", status.IndexedChunks)
	}
	if status.GraphNodes != 2 || status.GraphEdges != 1 {
		t.Fatalf("graph stats mismatch: nodes=%d edges=%d", status.GraphNodes, status.GraphEdges)
	}
	if !status.BM25Ready || status.BM25Empty {
		t.Fatalf("BM25 status mismatch: ready=%t empty=%t", status.BM25Ready, status.BM25Empty)
	}
	if !strings.Contains(status.LastIndexSummary, "incremental: indexed=2 deleted=1") {
		t.Fatalf("last index summary mismatch: %s", status.LastIndexSummary)
	}
}

func TestStatusReportsEmptyBM25(t *testing.T) {
	eng := &Engine{
		bm25:      search.NewBM25(),
		callGraph: graph.NewCallGraph(),
	}

	status := eng.Status()
	if status.BM25Ready {
		t.Fatal("empty BM25 should not be ready")
	}
	if !status.BM25Empty {
		t.Fatal("expected BM25 empty status")
	}
	if status.LastIndexSummary != "unavailable" {
		t.Fatalf("expected unavailable summary, got %q", status.LastIndexSummary)
	}
}
