package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/robby031/smart-rag/pkg/graph"
)

func (e *Engine) impactQuery(_ context.Context, q Query, resp *Response) (*Response, error) {
	depth := q.MaxDepth
	if depth <= 0 {
		depth = 3
	}
	var impact []graph.ImpactResult
	// A package path always contains "/"; function IDs (e.g. "engine.New", "New")
	// may contain "." but never "/". The old heuristic routed "engine.New" to
	// PackageImpact (import graph) because of the "." — which always returned empty.
	if strings.Contains(q.Text, "/") {
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
