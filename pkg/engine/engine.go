package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/bagusdwiharianto/smart-rag/pkg/graph"
	"github.com/bagusdwiharianto/smart-rag/pkg/indexer"
	"github.com/bagusdwiharianto/smart-rag/pkg/search"
	"github.com/bagusdwiharianto/smart-rag/pkg/searcher"
	"github.com/bagusdwiharianto/smart-rag/pkg/storage"
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
	sparse      *search.SparseRetriever
	hybrid      *search.HybridSearch
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
	sparse := search.NewSparseRetriever()
	hybrid := search.NewHybridSearch(bm25, sparse, 0.5)
	cg := graph.NewPersistentCallGraph(graphStore)
	ig := graph.NewPersistentImportGraph(graphStore)

	return &Engine{
		chunker:     chunker,
		parser:      parser,
		tokenizer:   tokenizer,
		bm25:        bm25,
		sparse:      sparse,
		hybrid:      hybrid,
		astSearch:   searcher.NewASTSearch(),
		graph:       graph.NewGraph(cg, ig),
		callGraph:   cg,
		importGraph: ig,
		chunkStore:  chunkStore,
	}
}

func (e *Engine) IndexFile(ctx context.Context, filePath, src string) error {
	decls, fileInfo, err := e.parser.ParseFile(filePath, src)
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

	for _, ch := range chunks {
		tokens := e.tokenizer.Tokenize(ch.Content)
		freq := make(map[string]int)
		for tok, count := range tokens {
			freq[tok] = count
		}
		e.bm25.AddDocument(freq, ch.ID)

		tfidf := e.tokenizer.TFIDF(freq, nil)
		vec := make(map[string]float64)
		for k, v := range tfidf {
			vec[k] = v
		}
		e.sparse.AddDocument(vec, ch.ID)

		chunkType := fmt.Sprintf("%d", ch.ChunkType)

		storeMeta := storage.ChunkMeta{
			ID:         ch.ID,
			FilePath:   ch.FilePath,
			ChunkType:  chunkType,
			SymbolName: ch.SymbolName,
			Signature:  ch.Signature,
			StartLine:  ch.StartLine,
			EndLine:    ch.EndLine,
			Content:    ch.Content,
		}
		if err := e.chunkStore.Put(storeMeta); err != nil {
			return fmt.Errorf("store chunk %s: %w", ch.ID, err)
		}
	}

	if err := e.callGraph.ParseFile(filePath, src, fileInfo.Package); err != nil {
		return fmt.Errorf("callgraph: %w", err)
	}
	if err := e.importGraph.AddFile(fileInfo.Package, filePath, src); err != nil {
		return fmt.Errorf("import graph: %w", err)
	}

	return nil
}

func (e *Engine) FinalizeIndex() {
	e.bm25.Build()
	e.sparse.Build()
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

	tfidf := e.tokenizer.TFIDF(freq, nil)
	sparseQuery := make(map[string]float64)
	for k, v := range tfidf {
		sparseQuery[k] = v
	}

	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	hybridResults := e.hybrid.Search(freq, sparseQuery, topK)
	for _, hr := range hybridResults {
		r := Result{Score: hr.FinalScore}
		chunk, err := e.chunkStore.Get(hr.ChunkID)
		if err == nil && chunk != nil {
			if q.Language != "" && !strings.HasSuffix(chunk.FilePath, "."+q.Language) {
				continue
			}
			if q.File != "" && !strings.Contains(chunk.FilePath, q.File) {
				continue
			}
			r.Chunk = chunk
			r.Content = chunk.Content
		}
		resp.Results = append(resp.Results, r)
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

// Stats returns a map of internal metrics for the performance banner.
func (e *Engine) Stats() map[string]int {
	graphStats := e.callGraph.Stats()
	m := map[string]int{
		"chunks":      len(e.bm25.DocIDs),
		"graph_nodes": graphStats["nodes"],
		"graph_edges": graphStats["edges"],
	}
	return m
}
