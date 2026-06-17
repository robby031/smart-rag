package engine

import (
	"context"
)

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
