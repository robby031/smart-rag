package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/robby031/smart-rag/pkg/storage"
)

func (e *Engine) getContextPack(_ context.Context, q Query, resp *Response) (*Response, error) {
	primary, err := e.chunkStore.Get(q.Text)
	if err != nil {
		return nil, fmt.Errorf("context not found: %w", err)
	}

	sections := []contextSection{
		{
			name:    "primary",
			content: formatContextChunk("primary", primary),
		},
	}
	if nearby := e.contextNearby(primary); nearby != "" {
		sections = append(sections, contextSection{name: "nearby", content: nearby})
	}
	if related := e.contextRelated(primary); related != "" {
		sections = append(sections, contextSection{name: "related", content: related})
	}

	resp.Results = append(resp.Results, Result{
		Chunk:   primary,
		Content: buildContextPack(sections, q.MaxTokens),
	})
	return resp, nil
}

type contextSection struct {
	name    string
	content string
}

func buildContextPack(sections []contextSection, maxChars int) string {
	if maxChars <= 0 {
		var full strings.Builder
		for i, section := range sections {
			if i > 0 {
				full.WriteString("\n\n")
			}
			full.WriteString(section.content)
		}
		return full.String()
	}

	var out strings.Builder
	for i, section := range sections {
		separator := ""
		if i > 0 {
			separator = "\n\n"
		}
		remaining := maxChars - out.Len()
		if remaining <= len(separator) {
			break
		}
		if separator != "" {
			out.WriteString(separator)
			remaining -= len(separator)
		}
		if len(section.content) <= remaining {
			out.WriteString(section.content)
			continue
		}
		if section.name != "primary" {
			break
		}
		out.WriteString(truncateContext(section.content, remaining))
		break
	}
	return out.String()
}

func truncateContext(content string, maxChars int) string {
	const marker = "\n...[truncated]"
	if maxChars <= 0 {
		return ""
	}
	if len(content) <= maxChars {
		return content
	}
	if maxChars <= len(marker) {
		return content[:maxChars]
	}
	return content[:maxChars-len(marker)] + marker
}

func (e *Engine) contextNearby(primary *storage.ChunkMeta) string {
	chunks, err := e.chunkStore.GetAllByFile(primary.FilePath)
	if err != nil || len(chunks) == 0 {
		return ""
	}

	idx := -1
	for i, chunk := range chunks {
		if chunk.ID == primary.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		return ""
	}

	type nearbyChunk struct {
		label string
		chunk *storage.ChunkMeta
	}
	var nearby []nearbyChunk
	if idx > 0 {
		nearby = append(nearby, nearbyChunk{label: "previous", chunk: chunks[idx-1]})
	}
	if idx+1 < len(chunks) {
		nearby = append(nearby, nearbyChunk{label: "next", chunk: chunks[idx+1]})
	}
	if len(nearby) == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString("## nearby")
	for _, item := range nearby {
		out.WriteString("\n\n")
		out.WriteString(formatContextChunk(item.label, item.chunk))
	}
	return out.String()
}

func (e *Engine) contextRelated(primary *storage.ChunkMeta) string {
	symbol := primary.SymbolName
	if symbol == "" {
		return ""
	}

	var lines []string
	lines = append(lines, e.relatedDefinitions(symbol, primary.ID)...)
	if e.graph != nil {
		lines = append(lines, e.relatedGraph(symbol)...)
	}
	if len(lines) == 0 {
		return ""
	}

	sort.Strings(lines)
	lines = dedupeStrings(lines)
	if len(lines) > 12 {
		lines = lines[:12]
	}

	var out strings.Builder
	out.WriteString("## related")
	for _, line := range lines {
		out.WriteString("\n- ")
		out.WriteString(line)
	}
	return out.String()
}

func (e *Engine) relatedDefinitions(symbol, primaryID string) []string {
	defs, err := e.chunkStore.SearchBySymbol(symbol, typeDefChunkTypes)
	if err != nil {
		return nil
	}
	var lines []string
	for _, def := range defs {
		if def.ID == primaryID {
			continue
		}
		lines = append(lines, fmt.Sprintf("definition: %s (%s:%d-%d)", def.SymbolName, def.FilePath, def.StartLine, def.EndLine))
	}
	return lines
}

func (e *Engine) relatedGraph(symbol string) []string {
	var lines []string
	ids := []string{symbol}
	for _, node := range e.graph.SearchSymbol(symbol) {
		ids = append(ids, node.ID())
		lines = append(lines, fmt.Sprintf("definition: %s (%s:%d)", node.ID(), node.File, node.Line))
	}
	for _, id := range dedupeStrings(ids) {
		xref := e.graph.Xref(id)
		for _, caller := range xref.Callers {
			lines = append(lines, "caller: "+caller)
		}
		for _, callee := range xref.Callees {
			lines = append(lines, "callee: "+callee)
		}
		for _, ref := range xref.References {
			lines = append(lines, "reference: "+ref)
		}
	}
	return lines
}

func formatContextChunk(label string, chunk *storage.ChunkMeta) string {
	var out strings.Builder
	out.WriteString("## ")
	out.WriteString(label)
	out.WriteString("\n")
	out.WriteString(fmt.Sprintf("Chunk: %s (%s, lines %d-%d)", chunk.ID, chunk.FilePath, chunk.StartLine, chunk.EndLine))
	if chunk.SymbolName != "" {
		out.WriteString("\n")
		out.WriteString("Symbol: ")
		out.WriteString(chunk.SymbolName)
	}
	out.WriteString("\nContent:\n")
	out.WriteString(chunk.Content)
	return out.String()
}

func dedupeStrings(in []string) []string {
	seen := make(map[string]bool, len(in))
	out := in[:0]
	for _, s := range in {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
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

	chunks, err := e.chunkStore.GetAllByFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("file lookup failed: %w", err)
	}
	if len(chunks) == 0 {
		return nil, fmt.Errorf("file not found: %s", filePath)
	}

	var snippet []string
	var bestChunk *storage.ChunkMeta
	for _, chunk := range chunks {
		if chunk.EndLine < startLine || chunk.StartLine > endLine {
			continue
		}
		if bestChunk == nil {
			bestChunk = chunk
		}
		lines := strings.Split(chunk.Content, "\n")
		for i, line := range lines {
			lineNum := chunk.StartLine + i
			if lineNum >= startLine && lineNum <= endLine {
				snippet = append(snippet, line)
			}
		}
	}
	if bestChunk == nil {
		bestChunk = chunks[0]
	}
	resp.Results = append(resp.Results, Result{
		Content: strings.Join(snippet, "\n"),
		Chunk:   bestChunk,
	})
	return resp, nil
}
