package indexer

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/robby031/smart-rag/pkg/storage"
)

type IndexEngine interface {
	IndexFile(ctx context.Context, filePath, src string) error
	FinalizeIndex() error
}

type Syncer struct {
	engine     IndexEngine
	indexStore *storage.IndexStore
	repoDir    string
}

func NewSyncer(engine IndexEngine, indexStore *storage.IndexStore, repoDir string) *Syncer {
	return &Syncer{
		engine:     engine,
		indexStore: indexStore,
		repoDir:    repoDir,
	}
}

func (s *Syncer) Sync(ctx context.Context) (int, int, error) {
	changed, deleted, err := s.detectChanges()
	if err != nil {
		return 0, 0, fmt.Errorf("detect changes: %w", err)
	}

	indexed := 0
	for _, filePath := range changed {
		absPath := filepath.Join(s.repoDir, filePath)
		src, err := os.ReadFile(absPath)
		if err != nil {
			if os.IsNotExist(err) {
				s.indexStore.DeleteHash(filePath)
				continue
			}
			return indexed, len(deleted), fmt.Errorf("read %s: %w", filePath, err)
		}
		if err := s.engine.IndexFile(ctx, filePath, string(src)); err != nil {
			return indexed, len(deleted), fmt.Errorf("index %s: %w", filePath, err)
		}
		s.indexStore.SaveHash(filePath, storage.ContentHash(src))
		indexed++
	}
	for _, filePath := range deleted {
		s.indexStore.DeleteHash(filePath)
	}

	if err := s.engine.FinalizeIndex(); err != nil {
		return indexed, len(deleted), fmt.Errorf("finalize: %w", err)
	}

	meta := &storage.IndexMeta{LastUpdated: time.Now()}
	if m, err := s.indexStore.LoadMeta(); err == nil {
		meta.FileCount = m.FileCount + indexed - len(deleted)
	} else {
		meta.FileCount = indexed
	}
	meta.TotalChunks = meta.FileCount
	s.indexStore.SaveMeta(*meta)

	return indexed, len(deleted), nil
}

func (s *Syncer) detectChanges() (changed, deleted []string, err error) {
	changed, deleted, err = s.gitDiff()
	if err == nil {
		return
	}
	return s.hashCompare()
}

func (s *Syncer) gitDiff() (changed, deleted []string, err error) {
	headCmd := exec.Command("git", "-C", s.repoDir, "rev-parse", "HEAD")
	if _, err := headCmd.Output(); err != nil {
		return nil, nil, fmt.Errorf("not a git repo: %w", err)
	}
	if _, err := s.indexStore.LoadMeta(); err != nil {
		return s.allFiles(), nil, nil
	}

	diffCmd := exec.Command("git", "-C", s.repoDir, "diff", "--name-only", "HEAD~1", "HEAD")
	diffOut, err := diffCmd.Output()
	if err != nil {
		return s.allFiles(), nil, nil
	}

	unstagedCmd := exec.Command("git", "-C", s.repoDir, "diff", "--name-only", "HEAD")
	unstagedOut, _ := unstagedCmd.Output()

	changedSet := make(map[string]bool)
	for _, line := range strings.Split(string(diffOut), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			changedSet[line] = true
		}
	}
	for _, line := range strings.Split(string(unstagedOut), "\n") {
		if line = strings.TrimSpace(line); line != "" {
			changedSet[line] = true
		}
	}

	for f := range changedSet {
		if !isIndexableExt(filepath.Ext(f)) {
			continue
		}
		absPath := filepath.Join(s.repoDir, f)
		if _, err := os.Stat(absPath); os.IsNotExist(err) {
			deleted = append(deleted, f)
		} else if src, err := os.ReadFile(absPath); err == nil {
			changedHash, _ := s.indexStore.HasChanged(f, src)
			if changedHash {
				changed = append(changed, f)
			}
		}
	}
	return
}

func (s *Syncer) hashCompare() (changed, deleted []string, err error) {
	filepath.Walk(s.repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			name := info.Name()
			if name == "vendor" || name == "testdata" || (name != "." && name[0] == '.') {
				return filepath.SkipDir
			}
			return nil
		}
		if !isIndexableExt(filepath.Ext(path)) {
			return nil
		}
		relPath, _ := filepath.Rel(s.repoDir, path)
		src, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		hashChanged, _ := s.indexStore.HasChanged(relPath, src)
		if hashChanged {
			changed = append(changed, relPath)
		}
		return nil
	})
	return changed, nil, nil
}

func isIndexableExt(ext string) bool {
	switch ext {
	case ".go", ".js", ".jsx", ".ts", ".tsx", ".mjs", ".cjs":
		return true
	}
	return false
}

func (s *Syncer) allFiles() []string {
	var files []string
	filepath.Walk(s.repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			if info.Name()[0] == '.' && info.Name() != "." {
				return filepath.SkipDir
			}
			return nil
		}
		if !isIndexableExt(filepath.Ext(path)) {
			return nil
		}
		relPath, _ := filepath.Rel(s.repoDir, path)
		files = append(files, relPath)
		return nil
	})
	return files
}
