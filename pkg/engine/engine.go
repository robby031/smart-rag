package engine

import (
	"context"
	"fmt"
	"sync"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/search"
	"github.com/robby031/smart-rag/pkg/searcher"
	"github.com/robby031/smart-rag/pkg/storage"
)

type Engine struct {
	chunker          *indexer.Chunker
	parser           *indexer.Parser
	tokenizer        *indexer.Tokenizer
	bm25             *search.BM25
	astSearch        *searcher.ASTSearch
	graph            *graph.Graph
	callGraph        *graph.CallGraph
	importGraph      *graph.ImportGraph
	chunkStore       *storage.ChunkStore
	flowGraph        *dataflow.FlowGraphBuilder
	flowIndex        *dataflow.FlowIndex
	flowStore        *storage.FlowStore
	pruningMode      PruningMode
	indexDirty       bool
	indexMu          sync.RWMutex
	statusMu         sync.RWMutex
	runtimeInfo      RuntimeInfo
	lastIndexSummary IndexSummary
}

func New(kvStore *storage.Store, chunkStore *storage.ChunkStore, _ *storage.VectorDB, graphStore *storage.GraphStore) *Engine {
	chunker := indexer.NewChunker(512)
	parser := indexer.NewParser()
	tokenizer := indexer.NewTokenizer()
	bm25 := search.NewBM25()
	cg := graph.NewPersistentCallGraph(graphStore)
	ig := graph.NewPersistentImportGraph(graphStore)
	flowStore := storage.NewFlowStore(kvStore)
	flowGraph := dataflow.NewFlowGraphBuilder(cg)
	flowIndex := dataflow.NewFlowIndex()

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
		flowStore:   flowStore,
		flowGraph:   flowGraph,
		flowIndex:   flowIndex,
		pruningMode: PruningModeSoft,
	}
}

func (e *Engine) Query(ctx context.Context, q Query) (*Response, error) {
	e.indexMu.RLock()
	defer e.indexMu.RUnlock()

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
	case QueryTraceVariable:
		resp.Type = "trace_variable"
		return e.handleTraceVariable(q)
	case QueryFunctionFlow:
		resp.Type = "function_dataflow"
		return e.handleFunctionFlow(q)
	case QueryTypeProvenance:
		resp.Type = "type_flow"
		return e.handleTypeProvenance(q)
	case QueryVariableSearch:
		resp.Type = "variable_search"
		return e.handleVariableSearch(q)
	case QueryDynamicFlow:
		resp.Type = "dynamic_flow"
		return e.handleDynamicFlow(q)
	default:
		return nil, fmt.Errorf("unknown query type: %v", q.Type)
	}
}

func (e *Engine) Stats() map[string]int {
	e.indexMu.RLock()
	defer e.indexMu.RUnlock()
	graphStats := e.callGraph.Stats()
	m := map[string]int{
		"chunks":      len(e.bm25.DocIDs),
		"graph_nodes": graphStats["nodes"],
		"graph_edges": graphStats["edges"],
	}
	return m
}
