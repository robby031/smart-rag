package engine

import (
	"context"
	"path/filepath"
	"testing"

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
