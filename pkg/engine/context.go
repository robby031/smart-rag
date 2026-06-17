package engine

import (
	"context"
	"fmt"
	"strings"
)

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
