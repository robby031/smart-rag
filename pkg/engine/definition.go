package engine

import (
	"context"
	"fmt"

	"github.com/robby031/smart-rag/pkg/indexer"
)

var typeDefChunkTypes = []string{
	fmt.Sprintf("%d", indexer.ChunkStruct),
	fmt.Sprintf("%d", indexer.ChunkInterface),
	fmt.Sprintf("%d", indexer.ChunkTypeDecl),
	fmt.Sprintf("%d", indexer.ChunkClass),
	fmt.Sprintf("%d", indexer.ChunkEnum),
}

func (e *Engine) findDefinition(_ context.Context, q Query, resp *Response) (*Response, error) {
	typeDefs, _ := e.chunkStore.SearchBySymbol(q.Text, typeDefChunkTypes)
	for _, ch := range typeDefs {
		label := typeLabel(ch.ChunkType)
		resp.Results = append(resp.Results, Result{
			Content: fmt.Sprintf("[%s] %s (%s:%d-%d)", label, ch.SymbolName, ch.FilePath, ch.StartLine, ch.EndLine),
		})
	}

	nodes := e.graph.SearchSymbol(q.Text)
	for _, node := range nodes {
		label := "func"
		if node.Recv != "" {
			label = "method"
		}
		resp.Results = append(resp.Results, Result{
			Node:    node,
			Content: fmt.Sprintf("[%s] %s (%s:%d)", label, node.ID(), node.File, node.Line),
		})
	}

	if len(resp.Results) == 0 {
		resp.Results = append(resp.Results, Result{Content: "no definition found"})
	}
	return resp, nil
}

func typeLabel(chunkType string) string {
	switch chunkType {
	case fmt.Sprintf("%d", indexer.ChunkStruct):
		return "struct"
	case fmt.Sprintf("%d", indexer.ChunkInterface):
		return "interface"
	case fmt.Sprintf("%d", indexer.ChunkClass):
		return "class"
	case fmt.Sprintf("%d", indexer.ChunkEnum):
		return "enum"
	default:
		return "type"
	}
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
