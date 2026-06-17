package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/searcher"
	"github.com/robby031/smart-rag/pkg/storage"
)

func (e *Engine) IndexFile(ctx context.Context, filePath, src string) error {
	return e.indexFileWith(filePath, src, e.bm25.AddDocument, e.chunkStore.PutAll)
}

func (e *Engine) indexFileWith(
	filePath, src string,
	addDoc func(map[string]int, string),
	chunkSink func([]storage.ChunkMeta) error,
) error {
	if err := e.chunkStore.DeleteByFile(filePath); err != nil {
		return fmt.Errorf("delete stale chunks %s: %w", filePath, err)
	}

	astFile, decls, fileInfo, err := e.parser.ParseFile(filePath, src)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}
	indexer.SetContent(decls, src)

	meta := indexer.FileMeta{
		Package: fileInfo.Package,
		Imports: fileInfo.Imports,
		IsTest:  fileInfo.IsTest,
	}
	chunks := e.chunker.Chunk(decls, filePath, meta)

	storeMetas := make([]storage.ChunkMeta, 0, len(chunks))
	for _, ch := range chunks {
		tokens := e.tokenizer.Tokenize(ch.Content)
		freq := make(map[string]int)
		for tok, count := range tokens {
			freq[tok] = count
		}
		addDoc(freq, ch.ID)

		storeMetas = append(storeMetas, storage.ChunkMeta{
			ID:         ch.ID,
			FilePath:   ch.FilePath,
			ChunkType:  fmt.Sprintf("%d", ch.ChunkType),
			SymbolName: ch.SymbolName,
			Signature:  ch.Signature,
			StartLine:  ch.StartLine,
			EndLine:    ch.EndLine,
			Content:    ch.Content,
		})
	}
	if err := chunkSink(storeMetas); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}

	e.callGraph.DeleteByFile(filePath)

	if err := e.callGraph.ParseAST(astFile, e.parser.FileSet(), filePath, fileInfo.Package); err != nil {
		return fmt.Errorf("callgraph: %w", err)
	}
	if err := e.importGraph.AddAST(fileInfo.Package, astFile); err != nil {
		return fmt.Errorf("import graph: %w", err)
	}

	return nil
}

func (e *Engine) IndexDir(ctx context.Context, repoDir string, workers int) error {
	if workers <= 0 {
		workers = runtime.NumCPU()
	}
	paths, err := searcher.WalkFiles(repoDir, 0)
	if err != nil {
		return fmt.Errorf("walk %s: %w", repoDir, err)
	}

	work := make(chan string, len(paths))
	for _, p := range paths {
		work <- p
	}
	close(work)

	type workerState struct {
		chunks []storage.ChunkMeta
	}
	states := make([]*workerState, workers)
	for i := range states {
		states[i] = &workerState{}
	}

	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)

	for i := range workers {
		wg.Add(1)
		ws := states[i]
		go func() {
			defer wg.Done()
			flushChunks := func() error {
				if len(ws.chunks) == 0 {
					return nil
				}
				err := e.chunkStore.PutAll(ws.chunks)

				ws.chunks = nil
				return err
			}
			sink := func(metas []storage.ChunkMeta) error {
				ws.chunks = append(ws.chunks, metas...)
				if len(ws.chunks) >= 2000 {
					return flushChunks()
				}
				return nil
			}
			for path := range work {
				src, err := os.ReadFile(path)
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("read %s: %w", path, err)
					}
					mu.Unlock()
					return
				}
				relPath, _ := filepath.Rel(repoDir, path)
				if err := e.indexFileWith(relPath, string(src), e.bm25.AddDocument, sink); err != nil {

					if strings.HasPrefix(err.Error(), "parse:") {
						continue
					}
					mu.Lock()
					if firstErr == nil {
						firstErr = fmt.Errorf("index %s: %w", path, err)
					}
					mu.Unlock()
					return
				}
			}
			if err := flushChunks(); err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				mu.Unlock()
			}
		}()
	}

	wg.Wait()
	return firstErr
}

func (e *Engine) FinalizeIndex() error {
	if e.bm25.IsEmpty() {
		if err := e.warmupBM25(); err != nil {
			return fmt.Errorf("warmup BM25: %w", err)
		}
	}
	e.bm25.Build()
	if err := e.callGraph.Flush(); err != nil {
		return fmt.Errorf("flush call graph: %w", err)
	}
	e.callGraph.BuildInEdges()
	if err := e.refreshChunkReachability(); err != nil {
		return fmt.Errorf("refresh chunk reachability: %w", err)
	}
	return e.importGraph.Flush()
}

func (e *Engine) warmupBM25() error {
	chunks, err := e.chunkStore.GetAll()
	if err != nil {
		return err
	}
	for _, ch := range chunks {
		tokens := e.tokenizer.Tokenize(ch.Content)
		freq := make(map[string]int)
		for tok, count := range tokens {
			freq[tok] = count
		}
		e.bm25.AddDocument(freq, ch.ID)
	}
	return nil
}
