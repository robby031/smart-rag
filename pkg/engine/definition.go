package engine

import (
	"context"
	"fmt"
)

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
