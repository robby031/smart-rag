package engine

import (
	"fmt"
	"strings"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

type TraceStep struct {
	Step    int    `json:"step"`
	Kind    string `json:"kind"`
	VarName string `json:"var_name"`
	VarType string `json:"var_type,omitempty"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	FuncID  string `json:"func_id,omitempty"`
	Code    string `json:"code,omitempty"`
	ChunkID string `json:"chunk_id,omitempty"`
}

type TraceResult struct {
	Variable   string      `json:"variable"`
	Trace      []TraceStep `json:"trace"`
	Provenance *TraceStep  `json:"provenance,omitempty"`
}

func (e *Engine) handleTraceVariable(q Query) (*Response, error) {
	vars := e.flowIndex.ByVariableName(q.Text)
	if len(vars) == 0 {
		return &Response{
			Query: q.Text,
			Type:  "trace_variable",
			Results: []Result{
				{Content: fmt.Sprintf("variable %q not found in flow index", q.Text)},
			},
		}, nil
	}

	matched := vars
	if q.File != "" {
		var filtered []*dataflow.Variable
		for _, v := range vars {
			if strings.Contains(v.File, q.File) {
				filtered = append(filtered, v)
			}
		}
		if len(filtered) > 0 {
			matched = filtered
		}
	}

	maxDepth := q.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 5
	}

	var allResults []Result
	for _, v := range matched {
		trace, provenance := e.traceBFS(v.Name, maxDepth)
		result := TraceResult{
			Variable:   v.Name,
			Trace:      trace,
			Provenance: provenance,
		}
		allResults = append(allResults, Result{Content: fmt.Sprintf("%+v", result)})
	}

	resp := &Response{
		Query:   q.Text,
		Type:    "trace_variable",
		Results: allResults,
	}
	return resp, nil
}

type bfsItem struct {
	name   string
	depth  int
	funcID string
}

func (e *Engine) traceBFS(varName string, maxDepth int) ([]TraceStep, *TraceStep) {
	visited := make(map[string]bool)
	queue := []bfsItem{{name: varName, depth: 0}}
	var steps []TraceStep
	var provenance *TraceStep

	for len(queue) > 0 && len(steps) < 100 {
		cur := queue[0]
		queue = queue[1:]

		if visited[cur.name] {
			continue
		}
		visited[cur.name] = true

		if cur.depth > maxDepth {
			continue
		}

		vars := e.flowIndex.ByVariableName(cur.name)
		if len(vars) == 0 {
			continue
		}
		v := vars[0]

		defID := defIDForVar(v)
		chain := e.flowIndex.GetChain(defID)
		if chain == nil {
			steps = append(steps, TraceStep{
				Step:    len(steps) + 1,
				Kind:    "definition",
				VarName: v.Name,
				VarType: v.Type,
				File:    v.File,
				Line:    v.DefLine,
			})
			continue
		}

		steps = append(steps, TraceStep{
			Step:    len(steps) + 1,
			Kind:    "definition",
			VarName: chain.Def.Variable,
			File:    chain.Def.File,
			Line:    chain.Def.StartLine,
			ChunkID: chain.Def.ChunkID,
		})

		for _, use := range chain.Uses {
			stepKind := useKindString(use.Kind)
			steps = append(steps, TraceStep{
				Step:    len(steps) + 1,
				Kind:    stepKind,
				VarName: use.Variable,
				File:    use.File,
				Line:    use.Line,
				FuncID:  use.FuncID,
			})

			if use.Kind == dataflow.UseCallArg && use.FuncID != "" {
				calleeVars := e.flowIndex.ByFunction(use.FuncID)
				for _, cv := range calleeVars {
					if !visited[cv.Name] {
						queue = append(queue, bfsItem{name: cv.Name, depth: cur.depth + 1, funcID: use.FuncID})
					}
				}
			}
		}

		if provenance == nil {
			provenance = e.findProvenance(cur.name)
		}
	}

	return steps, provenance
}

func (e *Engine) findProvenance(varName string) *TraceStep {
	nodes := e.graph.SearchSymbol(varName)
	if len(nodes) == 0 {
		return nil
	}

	callers := e.callGraph.Callers(nodes[0].ID())
	if len(callers) == 0 {
		return nil
	}

	return &TraceStep{
		Step:    0,
		Kind:    "provenance",
		VarName: varName,
		FuncID:  callers[0],
	}
}

func defIDForVar(v *dataflow.Variable) string {
	return fmt.Sprintf("%s.%s:%d:%s", v.Pkg, v.File, v.DefLine, v.Name)
}

func useKindString(kind dataflow.UseKind) string {
	switch kind {
	case dataflow.UseRead:
		return "read"
	case dataflow.UseWrite:
		return "write"
	case dataflow.UseCallArg:
		return "call_arg"
	case dataflow.UseReturn:
		return "return"
	default:
		return "unknown"
	}
}
