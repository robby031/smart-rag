package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/robby031/smart-rag/pkg/engine"
)

// SyncFunc triggers an incremental reindex of the repository.
type SyncFunc func(ctx context.Context) (indexed, deleted int, err error)

type SmartRAGServer struct {
	mcpServer *server.MCPServer
	engine    *engine.Engine
	syncFn    SyncFunc
}

func NewServer(e *engine.Engine, version string, syncFn SyncFunc) *SmartRAGServer {
	serverVersion := version
	if serverVersion == "" {
		serverVersion = "dev"
	}
	s := &SmartRAGServer{
		mcpServer: server.NewMCPServer("smart-rag", serverVersion),
		engine:    e,
		syncFn:    syncFn,
	}
	s.registerTools()
	return s
}

func (s *SmartRAGServer) Serve(transport string) error {
	return server.ServeStdio(s.mcpServer)
}

func (s *SmartRAGServer) registerTools() {
	s.mcpServer.AddTool(mcp.NewTool("rag_status",
		mcp.WithDescription("Show smart-rag health, index, graph, BM25, runtime path, and last sync status."),
	), s.handleRAGStatus)

	if s.syncFn != nil {
		s.mcpServer.AddTool(mcp.NewTool("reindex",
			mcp.WithDescription("Trigger an incremental reindex of the repository. Call this after large refactors or when search results seem stale. Returns how many files were indexed and removed."),
		), s.handleReindex)
	}

	s.mcpServer.AddTool(mcp.NewTool("search_code",
		mcp.WithDescription("Ranked BM25 code search with deterministic tie-breakers, lightweight boosts, and language/path filters."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithNumber("top_k", mcp.Description("Number of results (default 10)")),
		mcp.WithString("language", mcp.Description("Filter by language extension (e.g. go, py, ts)")),
		mcp.WithString("path", mcp.Description("Filter by file path pattern")),
	), s.handleSearchCode)

	s.mcpServer.AddTool(mcp.NewTool("find_definition",
		mcp.WithDescription("Find where a symbol is defined. Supports Go and JavaScript/TypeScript (functions, classes, types, enums, interfaces)."),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Symbol name (e.g. UserService, format, UserId)")),
	), s.handleFindDefinition)

	s.mcpServer.AddTool(mcp.NewTool("find_references",
		mcp.WithDescription("Find all locations where a symbol is used across the codebase (Go and JS/TS)"),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Symbol name to find references for")),
	), s.handleFindReferences)

	s.mcpServer.AddTool(mcp.NewTool("get_callers",
		mcp.WithDescription("List all functions that call the specified function. Supports Go and JS/TS."),
		mcp.WithString("function", mcp.Required(), mcp.Description("Function ID — Go: pkg.FuncName or pkg.(Recv).Method; JS/TS: module.funcName or module.(ClassName).method")),
	), s.handleGetCallers)

	s.mcpServer.AddTool(mcp.NewTool("get_callees",
		mcp.WithDescription("List all functions called by the specified function. Supports Go and JS/TS."),
		mcp.WithString("function", mcp.Required(), mcp.Description("Function ID — Go: pkg.FuncName or pkg.(Recv).Method; JS/TS: module.funcName or module.(ClassName).method")),
	), s.handleGetCallees)

	s.mcpServer.AddTool(mcp.NewTool("impact_analysis",
		mcp.WithDescription("Analyze blast radius: trace transitive impact of changing a function or package. Supports Go and JS/TS."),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Function or package/module name")),
		mcp.WithNumber("depth", mcp.Description("Traversal depth (default 3)")),
	), s.handleImpactAnalysis)

	s.mcpServer.AddTool(mcp.NewTool("get_context_pack",
		mcp.WithDescription("Retrieve full context for a code chunk, budget-limited for AI consumption"),
		mcp.WithString("chunk_id", mcp.Required(), mcp.Description("Chunk ID (e.g. path/file.go:1-42 or path/file.ts:1-42)")),
		mcp.WithNumber("max_tokens", mcp.Description("Max characters/tokens to return (default full)")),
		mcp.WithString("include_variables", mcp.Description("Include variable info (default: true)")),
		mcp.WithString("include_dataflow", mcp.Description("Include dataflow info (default: false)")),
	), s.handleGetContextPack)

	s.mcpServer.AddTool(mcp.NewTool("read_snippet",
		mcp.WithDescription("Read a specific code snippet at a given file:line location"),
		mcp.WithString("location", mcp.Required(), mcp.Description("File:line or file:start-end (e.g. main.go:10-25)")),
	), s.handleReadSnippet)

	s.mcpServer.AddTool(mcp.NewTool("trace_variable",
		mcp.WithDescription("Trace a variable through its def-use chain. "+
			"Shows where a variable is defined, how it is modified, and where it is used."),
		mcp.WithString("variable", mcp.Required(), mcp.Description("Variable name to trace")),
		mcp.WithString("location", mcp.Description("Optional anchor location (file:line) to disambiguate")),
		mcp.WithNumber("depth", mcp.Description("Maximum trace depth (default 5)")),
	), s.handleTraceVariable)

	s.mcpServer.AddTool(mcp.NewTool("function_dataflow",
		mcp.WithDescription("Show data flow inside a function: inputs (parameters), internal variables, and outputs (return values)."),
		mcp.WithString("function", mcp.Required(), mcp.Description("Function name (e.g. pkg.FuncName)")),
		mcp.WithNumber("depth", mcp.Description("Trace depth (default 1)")),
	), s.handleFunctionFlow)

	s.mcpServer.AddTool(mcp.NewTool("type_flow",
		mcp.WithDescription("Trace how a type is used across the codebase. "+
			"Forward trace: functions using the type. Backward trace: types containing this type."),
		mcp.WithString("type_name", mcp.Required(), mcp.Description("Type name (e.g. User, Config)")),
		mcp.WithNumber("depth", mcp.Description("Maximum depth (default 3)")),
	), s.handleTypeFlow)

	s.mcpServer.AddTool(mcp.NewTool("variable_search",
		mcp.WithDescription("Search for variables semantically across the codebase. "+
			"Supports exact match, fuzzy match, and type-based search. Results sorted by relevance."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithNumber("top_k", mcp.Description("Maximum results (default 10)")),
	), s.handleVariableSearch)

	s.mcpServer.AddTool(mcp.NewTool("trace_runtime",
		mcp.WithDescription("Show runtime trace data for a function or variable. "+
			"Data is collected by running tests with automatic instrumentation."),
		mcp.WithString("function", mcp.Required(), mcp.Description("Function or variable name")),
		mcp.WithString("test_pattern", mcp.Description("Test pattern (default: ./...)")),
	), s.handleTraceRuntime)
}

func (s *SmartRAGServer) handleRAGStatus(_ context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	return mcp.NewToolResultText(formatStatus(s.engine.Status())), nil
}

func (s *SmartRAGServer) handleReindex(ctx context.Context, _ mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	indexed, deleted, err := s.syncFn(ctx)
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("reindex failed: %v", err)), nil
	}
	return mcp.NewToolResultText(fmt.Sprintf("Reindex complete: %d files indexed, %d removed.", indexed, deleted)), nil
}

func (s *SmartRAGServer) handleSearchCode(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := req.Params.Arguments["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultText("query is required"), nil
	}
	topK := 10
	if v, ok := req.Params.Arguments["top_k"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			topK = int(f)
			if topK > 200 {
				topK = 200
			}
		}
	}
	language, _ := req.Params.Arguments["language"].(string)
	path, _ := req.Params.Arguments["path"].(string)

	resp, err := s.engine.Query(ctx, engine.Query{
		Type:     engine.QuerySearch,
		Text:     query,
		TopK:     topK,
		Language: language,
		File:     path,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("search failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleFindDefinition(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol, ok := req.Params.Arguments["symbol"].(string)
	if !ok || symbol == "" {
		return mcp.NewToolResultText("symbol is required"), nil
	}
	resp, err := s.engine.Query(ctx, engine.Query{Type: engine.QueryDefinition, Text: symbol})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("definition lookup failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleFindReferences(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol, ok := req.Params.Arguments["symbol"].(string)
	if !ok || symbol == "" {
		return mcp.NewToolResultText("symbol is required"), nil
	}
	resp, err := s.engine.Query(ctx, engine.Query{Type: engine.QueryReferences, Text: symbol})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("references lookup failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleGetCallers(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fn, ok := req.Params.Arguments["function"].(string)
	if !ok || fn == "" {
		return mcp.NewToolResultText("function is required"), nil
	}
	resp, err := s.engine.Query(ctx, engine.Query{Type: engine.QueryCallers, Text: fn})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("callers lookup failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleGetCallees(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fn, ok := req.Params.Arguments["function"].(string)
	if !ok || fn == "" {
		return mcp.NewToolResultText("function is required"), nil
	}
	resp, err := s.engine.Query(ctx, engine.Query{Type: engine.QueryCallees, Text: fn})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("callees lookup failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleImpactAnalysis(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	symbol, ok := req.Params.Arguments["symbol"].(string)
	if !ok || symbol == "" {
		return mcp.NewToolResultText("symbol is required"), nil
	}
	depth := 3
	if v, ok := req.Params.Arguments["depth"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			depth = int(f)
			if depth > 10 {
				depth = 10
			}
		}
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type:     engine.QueryImpact,
		Text:     symbol,
		MaxDepth: depth,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("impact analysis failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleGetContextPack(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	chunkID, ok := req.Params.Arguments["chunk_id"].(string)
	if !ok || chunkID == "" {
		return mcp.NewToolResultText("chunk_id is required"), nil
	}
	maxTokens := 0
	if v, ok := req.Params.Arguments["max_tokens"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			maxTokens = int(f)
		}
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type:      engine.QueryContextPack,
		Text:      chunkID,
		MaxTokens: maxTokens,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("get context failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleReadSnippet(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	location, ok := req.Params.Arguments["location"].(string)
	if !ok || location == "" {
		return mcp.NewToolResultText("location is required"), nil
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type: engine.QueryReadSnippet,
		Text: location,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("read snippet failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleTraceVariable(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	variable, ok := req.Params.Arguments["variable"].(string)
	if !ok || variable == "" {
		return mcp.NewToolResultText("variable is required"), nil
	}
	location, _ := req.Params.Arguments["location"].(string)
	depth := 5
	if v, ok := req.Params.Arguments["depth"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			depth = int(f)
			if depth > 20 {
				depth = 20
			}
		}
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type:     engine.QueryTraceVariable,
		Text:     variable,
		File:     location,
		MaxDepth: depth,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("trace_variable failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleFunctionFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fn, ok := req.Params.Arguments["function"].(string)
	if !ok || fn == "" {
		return mcp.NewToolResultText("function is required"), nil
	}
	depth := 1
	if v, ok := req.Params.Arguments["depth"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			depth = int(f)
			if depth > 5 {
				depth = 5
			}
		}
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type:     engine.QueryFunctionFlow,
		Text:     fn,
		MaxDepth: depth,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("function_dataflow failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleTypeFlow(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	typeName, ok := req.Params.Arguments["type_name"].(string)
	if !ok || typeName == "" {
		return mcp.NewToolResultText("type_name is required"), nil
	}
	depth := 3
	if v, ok := req.Params.Arguments["depth"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			depth = int(f)
			if depth > 10 {
				depth = 10
			}
		}
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type:     engine.QueryTypeProvenance,
		Text:     typeName,
		MaxDepth: depth,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("type_flow failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleVariableSearch(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	query, ok := req.Params.Arguments["query"].(string)
	if !ok || query == "" {
		return mcp.NewToolResultText("query is required"), nil
	}
	topK := 10
	if v, ok := req.Params.Arguments["top_k"]; ok {
		if f, ok := v.(float64); ok && f > 0 {
			topK = int(f)
			if topK > 100 {
				topK = 100
			}
		}
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type: engine.QueryVariableSearch,
		Text: query,
		TopK: topK,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("variable_search failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func (s *SmartRAGServer) handleTraceRuntime(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	fn, ok := req.Params.Arguments["function"].(string)
	if !ok || fn == "" {
		return mcp.NewToolResultText("function is required"), nil
	}
	testPattern, _ := req.Params.Arguments["test_pattern"].(string)
	if testPattern == "" {
		testPattern = "./..."
	}
	resp, err := s.engine.Query(ctx, engine.Query{
		Type: engine.QueryDynamicFlow,
		Text: fn,
		File: testPattern,
	})
	if err != nil {
		return mcp.NewToolResultText(fmt.Sprintf("trace_runtime failed: %v", err)), nil
	}
	return mcp.NewToolResultText(formatResults(resp)), nil
}

func formatResults(resp *engine.Response) string {
	if resp == nil || len(resp.Results) == 0 {
		return "No results found."
	}
	var out strings.Builder
	for idx, r := range resp.Results {
		out.WriteString(fmt.Sprintf("[%d] ", idx+1))
		if r.Node != nil {
			out.WriteString(fmt.Sprintf("Node: %s (%s:%d)\n", r.Node.ID(), r.Node.File, r.Node.Line))
		}
		if r.Chunk != nil {
			out.WriteString(fmt.Sprintf("Chunk: %s (%s, lines %d-%d)\n", r.Chunk.ID, r.Chunk.FilePath, r.Chunk.StartLine, r.Chunk.EndLine))
		}
		if r.Content != "" {
			out.WriteString(fmt.Sprintf("Content:\n%s\n", r.Content))
		}
		if len(r.Related) > 0 {
			out.WriteString(fmt.Sprintf("Related: %v\n", r.Related))
		}
		if r.Score > 0 {
			out.WriteString(fmt.Sprintf("Score: %.4f\n", r.Score))
		}
		out.WriteString("\n")
	}
	return out.String()
}

func formatStatus(status engine.Status) string {
	var out strings.Builder
	out.WriteString("smart-rag status\n")
	fmt.Fprintf(&out, "Version: %s\n", valueOrUnavailable(status.Version))
	fmt.Fprintf(&out, "Indexed chunks: %d\n", status.IndexedChunks)
	fmt.Fprintf(&out, "Graph nodes: %d\n", status.GraphNodes)
	fmt.Fprintf(&out, "Graph edges: %d\n", status.GraphEdges)
	fmt.Fprintf(&out, "BM25 ready: %t\n", status.BM25Ready)
	fmt.Fprintf(&out, "BM25 empty: %t\n", status.BM25Empty)
	fmt.Fprintf(&out, "Repo path: %s\n", valueOrUnavailable(status.RepoDir))
	fmt.Fprintf(&out, "DB path: %s\n", valueOrUnavailable(status.DBDir))
	fmt.Fprintf(&out, "Last index: %s", valueOrUnavailable(status.LastIndexSummary))
	return out.String()
}

func valueOrUnavailable(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unavailable"
	}
	return value
}
