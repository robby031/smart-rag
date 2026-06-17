package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/storage"
)

func searchEvalEngine(t *testing.T) *Engine {
	t.Helper()

	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	chunkStore := storage.NewChunkStore(kv)
	graphStore := storage.NewGraphStore(kv)
	eng := New(kv, chunkStore, nil, graphStore)

	repoDir := filepath.Join("testdata", "search_eval_repo")
	if err := eng.IndexDir(context.Background(), repoDir, 1); err != nil {
		t.Fatalf("IndexDir: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}
	return eng
}

func TestSearchEvalQualityCases(t *testing.T) {
	eng := searchEvalEngine(t)

	cases := []struct {
		name         string
		query        Query
		expectedFile string
		expectedSym  string
		minResults   int
	}{
		{
			name: "natural payment risk query",
			query: Query{
				Type: QuerySearch,
				Text: "ChargeCustomer payment risk scorer",
				TopK: 5,
			},
			expectedFile: "internal/payments/processor.go",
			expectedSym:  "ChargeCustomer",
			minResults:   2,
		},
		{
			name: "session validation query",
			query: Query{
				Type: QuerySearch,
				Text: "validate session token",
				TopK: 5,
			},
			expectedFile: "internal/auth/session.go",
			expectedSym:  "ValidateSession",
			minResults:   2,
		},
		{
			name: "exact refund symbol",
			query: Query{
				Type: QuerySearch,
				Text: "ProcessRefund",
				TopK: 5,
			},
			expectedFile: "internal/payments/processor.go",
			expectedSym:  "ProcessRefund",
			minResults:   2,
		},
		{
			name: "path fragment query",
			query: Query{
				Type: QuerySearch,
				Text: "internal/auth refresh access token",
				TopK: 5,
			},
			expectedFile: "internal/auth/session.go",
			expectedSym:  "RefreshAccessToken",
			minResults:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := eng.Query(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("Query search: %v", err)
			}
			if len(resp.Results) < tc.minResults {
				t.Fatalf("expected at least %d results, got %d", tc.minResults, len(resp.Results))
			}
			top := resp.Results[0].Chunk
			if top == nil {
				t.Fatal("top result has no chunk")
			}
			if top.FilePath != tc.expectedFile {
				t.Fatalf("top file mismatch: want %s, got %s", tc.expectedFile, top.FilePath)
			}
			if top.SymbolName != tc.expectedSym {
				t.Fatalf("top symbol mismatch: want %s, got %s", tc.expectedSym, top.SymbolName)
			}
			if len(resp.Results[0].Related) == 0 || !strings.Contains(resp.Results[0].Related[0], "score bm25=") {
				t.Fatalf("top result missing score explanation: %v", resp.Results[0].Related)
			}
		})
	}
}

func TestSearchEvalDeterminism(t *testing.T) {
	eng := searchEvalEngine(t)
	query := Query{
		Type: QuerySearch,
		Text: "payment refund workflow",
		TopK: 5,
	}

	var previous string
	for i := 0; i < 8; i++ {
		resp, err := eng.Query(context.Background(), query)
		if err != nil {
			t.Fatalf("Query search: %v", err)
		}
		if len(resp.Results) < 3 {
			t.Fatalf("expected at least 3 results, got %d", len(resp.Results))
		}
		current := resultIDs(resp)
		if previous != "" && current != previous {
			t.Fatalf("search order changed between runs:\nprevious: %s\ncurrent:  %s", previous, current)
		}
		previous = current
	}
}

func TestSearchEvalExactSymbolBeatsUsage(t *testing.T) {
	eng := searchEvalEngine(t)

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "ProcessRefund",
		TopK: 5,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) < 2 {
		t.Fatalf("expected symbol definition and usage results, got %d", len(resp.Results))
	}
	top := resp.Results[0].Chunk
	if top.FilePath != "internal/payments/processor.go" || top.SymbolName != "ProcessRefund" {
		t.Fatalf("exact symbol should rank definition first, got %s %s", top.FilePath, top.SymbolName)
	}
	if !strings.Contains(strings.Join(resp.Results[0].Related, "\n"), "boost exact_symbol=") {
		t.Fatalf("expected exact symbol boost explanation, got %v", resp.Results[0].Related)
	}
}

func TestSearchEvalFiltersPreserveRelevantResults(t *testing.T) {
	eng := searchEvalEngine(t)

	cases := []struct {
		name         string
		query        Query
		expectedFile string
		minResults   int
	}{
		{
			name: "language filter",
			query: Query{
				Type:     QuerySearch,
				Text:     "validate session token",
				TopK:     5,
				Language: "go",
			},
			expectedFile: "internal/auth/session.go",
			minResults:   1,
		},
		{
			name: "path filter",
			query: Query{
				Type: QuerySearch,
				Text: "payment refund workflow",
				TopK: 5,
				File: "internal/payments",
			},
			expectedFile: "internal/payments/reporting.go",
			minResults:   2,
		},
		{
			name: "language and path filter",
			query: Query{
				Type:     QuerySearch,
				Text:     "score payment risk",
				TopK:     5,
				Language: "go",
				File:     "internal/risk",
			},
			expectedFile: "internal/risk/scorer.go",
			minResults:   1,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := eng.Query(context.Background(), tc.query)
			if err != nil {
				t.Fatalf("Query search: %v", err)
			}
			if len(resp.Results) < tc.minResults {
				t.Fatalf("expected at least %d results, got %d", tc.minResults, len(resp.Results))
			}
			if resp.Results[0].Chunk.FilePath != tc.expectedFile {
				t.Fatalf("top file mismatch: want %s, got %s", tc.expectedFile, resp.Results[0].Chunk.FilePath)
			}
			for _, result := range resp.Results {
				if tc.query.Language != "" && !strings.HasSuffix(result.Chunk.FilePath, "."+tc.query.Language) {
					t.Fatalf("language filter leaked %s", result.Chunk.FilePath)
				}
				if tc.query.File != "" && !strings.Contains(result.Chunk.FilePath, tc.query.File) {
					t.Fatalf("path filter leaked %s", result.Chunk.FilePath)
				}
			}
		})
	}
}

func resultIDs(resp *Response) string {
	ids := make([]string, 0, len(resp.Results))
	for _, result := range resp.Results {
		if result.Chunk == nil {
			continue
		}
		ids = append(ids, result.Chunk.ID)
	}
	return strings.Join(ids, "|")
}
