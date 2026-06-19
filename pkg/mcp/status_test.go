package mcp

import (
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/engine"
)

func TestFormatStatus(t *testing.T) {
	out := formatStatus(engine.Status{
		Version:          "0.4.5",
		RepoDir:          "/repo",
		DBDir:            "/db",
		IndexedChunks:    12,
		GraphNodes:       3,
		GraphEdges:       4,
		BM25Ready:        true,
		BM25Empty:        false,
		LastIndexSummary: "incremental: indexed=2 deleted=1 updated_at=2026-06-18T10:11:12Z",
	})

	for _, want := range []string{
		"smart-rag status",
		"Version: 0.4.5",
		"Indexed chunks: 12",
		"Graph nodes: 3",
		"Graph edges: 4",
		"BM25 ready: true",
		"BM25 empty: false",
		"Repo path: /repo",
		"DB path: /db",
		"Last index: incremental: indexed=2 deleted=1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("expected formatted status to contain %q:\n%s", want, out)
		}
	}
}
