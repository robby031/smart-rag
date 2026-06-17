package engine

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/search"
	"github.com/robby031/smart-rag/pkg/searcher"
	"github.com/robby031/smart-rag/pkg/storage"
)

type QueryType int

const (
	QuerySearch QueryType = iota
	QueryDefinition
	QueryReferences
	QueryCallers
	QueryCallees
	QueryImpact
	QueryContextPack
	QueryReadSnippet
)

type Query struct {
	Type      QueryType
	Text      string
	File      string
	Language  string
	TopK      int
	MaxDepth  int
	MaxTokens int
}

type Result struct {
	Score   float64              `json:"score,omitempty"`
	Chunk   *storage.ChunkMeta   `json:"chunk,omitempty"`
	Node    *graph.Node          `json:"node,omitempty"`
	Impact  []graph.ImpactResult `json:"impact,omitempty"`
	Related []string             `json:"related,omitempty"`
	Content string               `json:"content,omitempty"`
}

type Response struct {
	Query   string   `json:"query"`
	Type    string   `json:"type"`
	Results []Result `json:"results"`
}

type Engine struct {
	chunker     *indexer.Chunker
	parser      *indexer.Parser
	tokenizer   *indexer.Tokenizer
	bm25        *search.BM25
	astSearch   *searcher.ASTSearch
	graph       *graph.Graph
	callGraph   *graph.CallGraph
	importGraph *graph.ImportGraph
	chunkStore  *storage.ChunkStore
}

func New(kvStore *storage.Store, chunkStore *storage.ChunkStore, _ *storage.VectorDB, graphStore *storage.GraphStore) *Engine {
	chunker := indexer.NewChunker(512)
	parser := indexer.NewParser()
	tokenizer := indexer.NewTokenizer()
	bm25 := search.NewBM25()
	cg := graph.NewPersistentCallGraph(graphStore)
	ig := graph.NewPersistentImportGraph(graphStore)

	return &Engine{
		chunker:     chunker,
		parser:      parser,
		tokenizer:   tokenizer,
		bm25:        bm25,
		astSearch:   searcher.NewASTSearch(),
		graph:       graph.NewGraph(cg, ig),
		callGraph:   cg,
		importGraph: ig,
		chunkStore:  chunkStore,
	}
}

func (e *Engine) IndexFile(ctx context.Context, filePath, src string) error {
	return e.indexFileWith(filePath, src, e.bm25.AddDocument, e.chunkStore.PutAll)
}

func (e *Engine) indexFileWith(
	filePath, src string,
	addDoc func(map[string]int, string),
	chunkSink func([]storage.ChunkMeta) error,
) error {
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
	e.bm25.Build()
	if err := e.callGraph.Flush(); err != nil {
		return fmt.Errorf("flush call graph: %w", err)
	}
	e.callGraph.BuildInEdges()
	return e.importGraph.Flush()
}

func (e *Engine) Query(ctx context.Context, q Query) (*Response, error) {
	resp := &Response{Query: q.Text}

	switch q.Type {
	case QuerySearch:
		resp.Type = "search_code"
		return e.search(ctx, q, resp)
	case QueryDefinition:
		resp.Type = "find_definition"
		return e.findDefinition(ctx, q, resp)
	case QueryReferences:
		resp.Type = "find_references"
		return e.findReferences(ctx, q, resp)
	case QueryCallers:
		resp.Type = "get_callers"
		return e.getCallers(ctx, q, resp)
	case QueryCallees:
		resp.Type = "get_callees"
		return e.getCallees(ctx, q, resp)
	case QueryImpact:
		resp.Type = "impact_analysis"
		return e.impactQuery(ctx, q, resp)
	case QueryContextPack:
		resp.Type = "get_context_pack"
		return e.getContextPack(ctx, q, resp)
	case QueryReadSnippet:
		resp.Type = "read_snippet"
		return e.readSnippet(ctx, q, resp)
	default:
		return nil, fmt.Errorf("unknown query type: %v", q.Type)
	}
}

func (e *Engine) search(_ context.Context, q Query, resp *Response) (*Response, error) {
	tokens := e.tokenizer.Tokenize(q.Text)
	freq := make(map[string]int)
	for tok, count := range tokens {
		freq[tok] = count
	}

	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	for _, sr := range e.bm25.Search(freq, topK) {
		chunk, err := e.chunkStore.Get(sr.ID)
		if err != nil || chunk == nil {
			continue
		}
		if q.Language != "" && !strings.HasSuffix(chunk.FilePath, "."+q.Language) {
			continue
		}
		if q.File != "" && !strings.Contains(chunk.FilePath, q.File) {
			continue
		}
		resp.Results = append(resp.Results, Result{Score: sr.Score, Chunk: chunk, Content: chunk.Content})
	}

	return resp, nil
}

func (e *Engine) findDefinition(_ context.Context, q Query, resp *Response) (*Response, error) {
	nodes := e.graph.SearchSymbol(q.Text)
	for _, node := range nodes {
		resp.Results = append(resp.Results, Result{Node: node, Content: fmt.Sprintf("%s (%s:%d)", node.ID(), node.File, node.Line)})
	}
	return resp, nil
}

func (e *Engine) findReferences(_ context.Context, q Query, resp *Response) (*Response, error) {
	xref := e.graph.Xref(q.Text)
	for _, ref := range xref.References {
		resp.Results = append(resp.Results, Result{Content: ref})
	}
	for _, def := range xref.Definitions {
		resp.Results = append(resp.Results, Result{Content: "def: " + def})
	}
	return resp, nil
}

func (e *Engine) getCallers(_ context.Context, q Query, resp *Response) (*Response, error) {
	callers := e.graph.Callers(q.Text)
	for _, c := range callers {
		resp.Results = append(resp.Results, Result{Content: c})
	}
	if len(resp.Results) == 0 {
		resp.Results = append(resp.Results, Result{Content: "no callers found"})
	}
	return resp, nil
}

func (e *Engine) getCallees(_ context.Context, q Query, resp *Response) (*Response, error) {
	callees := e.graph.Callees(q.Text)
	for _, c := range callees {
		resp.Results = append(resp.Results, Result{Content: c})
	}
	if len(resp.Results) == 0 {
		resp.Results = append(resp.Results, Result{Content: "no callees found"})
	}
	return resp, nil
}

func (e *Engine) impactQuery(_ context.Context, q Query, resp *Response) (*Response, error) {
	depth := q.MaxDepth
	if depth <= 0 {
		depth = 3
	}
	var impact []graph.ImpactResult
	if strings.Contains(q.Text, "/") || (strings.Contains(q.Text, ".") && !strings.Contains(q.Text, "(")) {
		impact = e.graph.PackageImpact(q.Text, depth)
	} else {
		impact = e.graph.ImpactFull(q.Text, depth)
	}
	for _, im := range impact {
		resp.Results = append(resp.Results, Result{Content: fmt.Sprintf("[%s] %s (depth %d)", im.Dir, im.ID, im.Depth)})
	}
	if len(resp.Results) == 0 {
		resp.Results = append(resp.Results, Result{Content: "no impact detected"})
	}
	return resp, nil
}

func (e *Engine) getContextPack(_ context.Context, q Query, resp *Response) (*Response, error) {
	chunk, err := e.chunkStore.Get(q.Text)
	if err != nil {
		return nil, fmt.Errorf("context not found: %w", err)
	}
	content := chunk.Content
	if q.MaxTokens > 0 && len(content) > q.MaxTokens {
		content = content[:q.MaxTokens]
	}
	resp.Results = append(resp.Results, Result{Chunk: chunk, Content: content})
	return resp, nil
}

func (e *Engine) readSnippet(_ context.Context, q Query, resp *Response) (*Response, error) {
	parts := strings.SplitN(q.Text, ":", 3)
	if len(parts) < 2 {
		return nil, fmt.Errorf("format: file:line or file:startLine-endLine")
	}
	filePath := parts[0]
	lineRange := parts[1]
	var startLine, endLine int
	if _, err := fmt.Sscanf(lineRange, "%d-%d", &startLine, &endLine); err != nil {
		if _, err2 := fmt.Sscanf(lineRange, "%d", &startLine); err2 != nil {
			return nil, fmt.Errorf("invalid line range: %s", lineRange)
		}
		endLine = startLine
	}

	chunk, err := e.chunkStore.Get(filePath)
	if err != nil {
		return nil, fmt.Errorf("file not found: %w", err)
	}
	lines := strings.Split(chunk.Content, "\n")
	offset := chunk.StartLine
	var snippet []string
	for i, line := range lines {
		lineNum := offset + i
		if lineNum >= startLine && lineNum <= endLine {
			snippet = append(snippet, line)
		}
	}
	resp.Results = append(resp.Results, Result{
		Content: strings.Join(snippet, "\n"),
		Chunk:   chunk,
	})
	return resp, nil
}

func (e *Engine) Stats() map[string]int {
	graphStats := e.callGraph.Stats()
	m := map[string]int{
		"chunks":      len(e.bm25.DocIDs),
		"graph_nodes": graphStats["nodes"],
		"graph_edges": graphStats["edges"],
	}
	return m
}
