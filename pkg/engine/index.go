package engine

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/searcher"
	"github.com/robby031/smart-rag/pkg/storage"
)

func (e *Engine) IndexFile(ctx context.Context, filePath, src string) error {
	e.indexMu.Lock()
	defer e.indexMu.Unlock()
	e.indexDirty = true
	return e.indexFileWith(filePath, src, e.bm25.AddDocument, e.chunkStore.PutAll)
}

func (e *Engine) indexFileWith(
	filePath, src string,
	addDoc func(map[string]int, string),
	chunkSink func([]storage.ChunkMeta) error,
) error {
	generated := isGeneratedSource(src)
	sink := chunkSink
	if generated {
		sink = func(metas []storage.ChunkMeta) error {
			for i := range metas {
				metas[i].SemanticRole = SemanticRoleBoilerplate
				metas[i].FoldReason = FoldReasonGeneratedCode
				metas[i].ContextWeight = generatedContextWeight
			}
			return chunkSink(metas)
		}
	}
	if indexer.IsJSLike(filePath) {
		return e.indexJSFileWith(filePath, src, addDoc, sink)
	}
	return e.indexGoFileWith(filePath, src, addDoc, sink)
}

func (e *Engine) indexGoFileWith(
	filePath, src string,
	addDoc func(map[string]int, string),
	chunkSink func([]storage.ChunkMeta) error,
) error {
	if err := e.chunkStore.DeleteByFile(filePath); err != nil {
		return fmt.Errorf("delete stale chunks %s: %w", filePath, err)
	}
	if e.flowStore != nil {
		e.flowStore.DeleteByFile(filePath)
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
	ve := indexer.NewVariableExtractor()
	chunks := e.chunker.ChunkWithVars(decls, filePath, meta, ve, src)

	storeMetas := make([]storage.ChunkMeta, 0, len(chunks))
	for _, ch := range chunks {
		tokens := e.tokenizer.Tokenize(ch.Content)
		freq := make(map[string]int)
		for tok, count := range tokens {
			freq[tok] = count
		}
		addDoc(freq, ch.ID)

		cm := storage.ChunkMeta{
			ID:         ch.ID,
			FilePath:   ch.FilePath,
			ChunkType:  fmt.Sprintf("%d", ch.ChunkType),
			SymbolName: ch.SymbolName,
			Signature:  ch.Signature,
			StartLine:  ch.StartLine,
			EndLine:    ch.EndLine,
			Content:    ch.Content,
		}
		storeMetas = append(storeMetas, cm)
	}
	if err := chunkSink(storeMetas); err != nil {
		return fmt.Errorf("store chunks: %w", err)
	}

	if e.flowGraph != nil && e.flowStore != nil {
		fg, err := e.flowGraph.BuildFromAST(astFile, e.parser.FileSet(), filePath, fileInfo.Package)
		if err == nil {
			for _, v := range fg.Variables {
				e.flowStore.SaveVariable(v)
			}
			for _, chain := range fg.DefUseMap {
				e.flowStore.SaveChain(chain)
			}
			for _, node := range fg.TypeNodes {
				e.flowStore.SaveTypeNode(node)
			}
			for _, edge := range fg.Edges {
				e.flowStore.SaveEdge(&edge)
			}
			e.flowIndex.BuildFromFlowGraph(fg)
		}
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

func (e *Engine) indexJSFileWith(
	filePath, src string,
	addDoc func(map[string]int, string),
	chunkSink func([]storage.ChunkMeta) error,
) error {
	if err := e.chunkStore.DeleteByFile(filePath); err != nil {
		return fmt.Errorf("delete stale chunks %s: %w", filePath, err)
	}
	if e.flowStore != nil {
		e.flowStore.DeleteByFile(filePath)
	}

	decls, fileInfo, err := indexer.ParseJSFile(filePath, src)
	if err != nil {
		return fmt.Errorf("parse js: %w", err)
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
	if err := e.callGraph.ParseJSAST(filePath, src, fileInfo.Package); err != nil {
		return fmt.Errorf("js callgraph: %w", err)
	}
	e.importGraph.AddImports(fileInfo.Package, fileInfo.Imports)
	return nil
}

func (e *Engine) IndexDir(ctx context.Context, repoDir string, workers int) error {
	e.indexMu.Lock()
	defer e.indexMu.Unlock()
	e.indexDirty = true

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
	e.indexMu.Lock()
	defer e.indexMu.Unlock()

	needsWarmup := e.bm25.IsEmpty()
	if !e.indexDirty && !needsWarmup {
		return nil
	}

	if needsWarmup {
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
	if _, err := e.applyHardPruning(); err != nil {
		return fmt.Errorf("hard prune chunks: %w", err)
	}
	if e.flowStore != nil && e.flowIndex != nil {
		fg := &dataflow.FlowGraph{
			Variables: make(map[string]*dataflow.Variable),
			Defs:      make(map[string]*dataflow.VarDef),
			Uses:      make(map[string]*dataflow.VarUse),
			DefUseMap: make(map[string]*dataflow.DefUseChain),
			TypeNodes: make(map[string]*dataflow.TypeFlowNode),
		}

		vars, _ := e.flowStore.LoadAllVariables()
		for _, v := range vars {
			fg.Variables[v.Name] = v
		}

		defs, _ := e.flowStore.LoadAllDefs()
		for _, d := range defs {
			fg.Defs[d.ID] = d
		}

		uses, _ := e.flowStore.LoadAllUses()
		for _, u := range uses {
			fg.Uses[u.ID] = u
		}

		edges, _ := e.flowStore.LoadAllEdges()
		fg.Edges = edges

		e.flowIndex.BuildFromFlowGraph(fg)
	}
	e.indexDirty = false
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
		maps.Copy(freq, tokens)
		e.bm25.AddDocument(freq, ch.ID)
	}
	return nil
}
