package storage_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/robby031/smart-rag/pkg/storage"
)

func TestChunkStoreNoStaleDuplicates(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	cs := storage.NewChunkStore(kv)

	filePath := "pkg/engine/engine.go"

	firstPass := []storage.ChunkMeta{
		{ID: filePath + ":14-25", FilePath: filePath, StartLine: 14, EndLine: 25,
			Content: "type Engine struct { oldField string }"},
		{ID: filePath + ":27-48", FilePath: filePath, StartLine: 27, EndLine: 48,
			Content: "func New() *Engine { return &Engine{} }"},
	}
	if err := cs.PutAll(firstPass); err != nil {
		t.Fatalf("first PutAll: %v", err)
	}

	chunks, err := cs.GetAllByFile(filePath)
	if err != nil {
		t.Fatalf("GetAllByFile after first pass: %v", err)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks after first pass, got %d", len(chunks))
	}

	if err := cs.DeleteByFile(filePath); err != nil {
		t.Fatalf("DeleteByFile: %v", err)
	}

	secondPass := []storage.ChunkMeta{
		{ID: filePath + ":14-24", FilePath: filePath, StartLine: 14, EndLine: 24,
			Content: "type Engine struct {}"},
		{ID: filePath + ":26-44", FilePath: filePath, StartLine: 26, EndLine: 44,
			Content: "func New() *Engine {}"},
	}
	if err := cs.PutAll(secondPass); err != nil {
		t.Fatalf("second PutAll: %v", err)
	}

	chunks, err = cs.GetAllByFile(filePath)
	if err != nil {
		t.Fatalf("GetAllByFile after second pass: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("expected 2 chunks after re-index, got %d (stale data present)", len(chunks))
		for _, c := range chunks {
			t.Logf("  chunk id=%s lines=%d-%d", c.ID, c.StartLine, c.EndLine)
		}
	}

	for _, c := range chunks {
		if c.StartLine == 14 && c.EndLine == 25 {
			t.Errorf("stale chunk %s still present after re-index", c.ID)
		}
	}
}

func TestDeleteWithPrefixAtomicity(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	cs := storage.NewChunkStore(kv)

	file := "pkg/foo/bar.go"
	otherFile := "pkg/foo/baz.go"

	all := []storage.ChunkMeta{
		{ID: file + ":1-10", FilePath: file, StartLine: 1, EndLine: 10, Content: "a"},
		{ID: file + ":11-20", FilePath: file, StartLine: 11, EndLine: 20, Content: "b"},
		{ID: otherFile + ":1-5", FilePath: otherFile, StartLine: 1, EndLine: 5, Content: "c"},
	}
	if err := cs.PutAll(all); err != nil {
		t.Fatalf("PutAll: %v", err)
	}

	if err := cs.DeleteByFile(file); err != nil {
		t.Fatalf("DeleteByFile: %v", err)
	}

	gone, err := cs.GetAllByFile(file)
	if err != nil {
		t.Fatalf("GetAllByFile target: %v", err)
	}
	if len(gone) != 0 {
		t.Errorf("expected 0 chunks for %s after delete, got %d", file, len(gone))
	}

	kept, err := cs.GetAllByFile(otherFile)
	if err != nil {
		t.Fatalf("GetAllByFile other: %v", err)
	}
	if len(kept) != 1 {
		t.Errorf("expected 1 chunk for %s, got %d", otherFile, len(kept))
	}

	_ = os.RemoveAll(dir)
}
