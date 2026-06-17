package mcp

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	"github.com/robby031/smart-rag/pkg/engine"
)

type SmartRAGServer struct {
	mcpServer *server.MCPServer
	engine    *engine.Engine
}

func NewServer(e *engine.Engine) *SmartRAGServer {
	s := &SmartRAGServer{
		mcpServer: server.NewMCPServer("smart-rag", "0.1.0"),
		engine:    e,
	}
	s.registerTools()
	return s
}

func (s *SmartRAGServer) Serve(transport string) error {
	return server.ServeStdio(s.mcpServer)
}

func (s *SmartRAGServer) registerTools() {
	s.mcpServer.AddTool(mcp.NewTool("search_code",
		mcp.WithDescription("Hybrid search code (BM25 + sparse vector). Supports language and path filters."),
		mcp.WithString("query", mcp.Required(), mcp.Description("Search query")),
		mcp.WithNumber("top_k", mcp.Description("Number of results (default 10)")),
		mcp.WithString("language", mcp.Description("Filter by language extension (e.g. go, py, ts)")),
		mcp.WithString("path", mcp.Description("Filter by file path pattern")),
	), s.handleSearchCode)

	s.mcpServer.AddTool(mcp.NewTool("find_definition",
		mcp.WithDescription("Go-to-definition: find where a symbol is defined"),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Symbol name (function, type, variable)")),
	), s.handleFindDefinition)

	s.mcpServer.AddTool(mcp.NewTool("find_references",
		mcp.WithDescription("Find all locations where a symbol is used across the codebase"),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Symbol name to find references for")),
	), s.handleFindReferences)

	s.mcpServer.AddTool(mcp.NewTool("get_callers",
		mcp.WithDescription("List all functions that call the specified function"),
		mcp.WithString("function", mcp.Required(), mcp.Description("Function ID (e.g. pkg.FuncName)")),
	), s.handleGetCallers)

	s.mcpServer.AddTool(mcp.NewTool("get_callees",
		mcp.WithDescription("List all functions called by the specified function"),
		mcp.WithString("function", mcp.Required(), mcp.Description("Function ID (e.g. pkg.FuncName)")),
	), s.handleGetCallees)

	s.mcpServer.AddTool(mcp.NewTool("impact_analysis",
		mcp.WithDescription("Analyze blast radius: trace transitive impact of changing a function or package"),
		mcp.WithString("symbol", mcp.Required(), mcp.Description("Function or package name")),
		mcp.WithNumber("depth", mcp.Description("Traversal depth (default 3)")),
	), s.handleImpactAnalysis)

	s.mcpServer.AddTool(mcp.NewTool("get_context_pack",
		mcp.WithDescription("Retrieve full context for a code chunk, budget-limited for AI consumption"),
		mcp.WithString("chunk_id", mcp.Required(), mcp.Description("Chunk ID (e.g. path/file.go:1-42)")),
		mcp.WithNumber("max_tokens", mcp.Description("Max characters/tokens to return (default full)")),
	), s.handleGetContextPack)

	s.mcpServer.AddTool(mcp.NewTool("read_snippet",
		mcp.WithDescription("Read a specific code snippet at a given file:line location"),
		mcp.WithString("location", mcp.Required(), mcp.Description("File:line or file:start-end (e.g. main.go:10-25)")),
	), s.handleReadSnippet)
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

func formatResults(resp *engine.Response) string {
	if resp == nil || len(resp.Results) == 0 {
		return "No results found."
	}
	var out strings.Builder
	for i, r := range resp.Results {
		out.WriteString(fmt.Sprintf("[%d] ", i+1))
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
		_ = i
		out.WriteString("\n")
	}
	return out.String()
}
