package engine

import (
	"fmt"
	"sort"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

type InputVar struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	Callers []string `json:"callers,omitempty"`
}

type InternalVar struct {
	Name       string     `json:"name"`
	Type       string     `json:"type"`
	DefLine    int        `json:"def_line"`
	UsageCount int        `json:"usage_count"`
	Chain      []UsageLoc `json:"chain"`
}

type UsageLoc struct {
	Kind string `json:"kind"`
	Line int    `json:"line"`
	File string `json:"file"`
}

type OutputVar struct {
	Name    string   `json:"name"`
	Type    string   `json:"type"`
	FuncID  string   `json:"func_id,omitempty"`
	Callees []string `json:"callees,omitempty"`
}

type FunctionFlowResult struct {
	FuncID      string        `json:"func_id"`
	File        string        `json:"file"`
	Signature   string        `json:"signature,omitempty"`
	Inputs      []InputVar    `json:"inputs"`
	Internals   []InternalVar `json:"internals"`
	Outputs     []OutputVar   `json:"outputs"`
	SideEffects []string      `json:"side_effects,omitempty"`
}

func (e *Engine) handleFunctionFlow(q Query) (*Response, error) {
	nodes := e.graph.SearchSymbol(q.Text)
	if len(nodes) == 0 {
		return &Response{
			Query: q.Text,
			Type:  "function_dataflow",
			Results: []Result{
				{Content: fmt.Sprintf("function %q not found", q.Text)},
			},
		}, nil
	}

	funcID := nodes[0].ID()
	vars := e.flowIndex.ByFunction(funcID)
	if len(vars) == 0 {
		return &Response{
			Query: q.Text,
			Type:  "function_dataflow",
			Results: []Result{
				{Content: fmt.Sprintf("no flow data for function %s", funcID)},
			},
		}, nil
	}

	result := FunctionFlowResult{
		FuncID:    funcID,
		File:      nodes[0].File,
		Signature: nodes[0].Name,
	}

	seenParam := make(map[string]bool)
	seenLocal := make(map[string]bool)

	for _, v := range vars {
		switch v.Scope {
		case dataflow.ScopeParam:
			if !seenParam[v.Name] {
				iv := InputVar{
					Name: v.Name,
					Type: v.Type,
				}
				callers := e.callGraph.Callers(funcID)
				for _, c := range callers {
					iv.Callers = append(iv.Callers, c)
				}
				sort.Strings(iv.Callers)
				result.Inputs = append(result.Inputs, iv)
				seenParam[v.Name] = true
			}
		case dataflow.ScopeLocal:
			if !seenLocal[v.Name] {
				iv := InternalVar{
					Name:    v.Name,
					Type:    v.Type,
					DefLine: v.DefLine,
				}
				defID := defIDForVar(v)
				chain := e.flowIndex.GetChain(defID)
				if chain != nil {
					iv.UsageCount = len(chain.Uses)
					for _, use := range chain.Uses {
						iv.Chain = append(iv.Chain, UsageLoc{
							Kind: useKindString(use.Kind),
							Line: use.Line,
							File: use.File,
						})
					}
				}
				result.Internals = append(result.Internals, iv)
				seenLocal[v.Name] = true
			}
		case dataflow.ScopeGlobal:
			result.SideEffects = append(result.SideEffects, v.Name)
		}
	}

	for _, v := range vars {
		if v.Scope == dataflow.ScopeParam {
			continue
		}
		defID := defIDForVar(v)
		chain := e.flowIndex.GetChain(defID)
		if chain == nil {
			continue
		}
		for _, use := range chain.Uses {
			if use.Kind == dataflow.UseReturn {
				result.Outputs = append(result.Outputs, OutputVar{
					Name:   v.Name,
					Type:   v.Type,
					FuncID: use.FuncID,
				})
				break
			}
		}
	}

	resp := &Response{
		Query: q.Text,
		Type:  "function_dataflow",
		Results: []Result{
			{Content: fmt.Sprintf("%+v", result)},
		},
	}
	return resp, nil
}
