package engine

import (
	"fmt"
)

type TypeDefLoc struct {
	File string `json:"file"`
	Line int    `json:"line"`
	Code string `json:"code,omitempty"`
}

type TypeUsage struct {
	Kind    string `json:"kind"`
	FuncID  string `json:"func_id"`
	File    string `json:"file"`
	Line    int    `json:"line"`
	ChunkID string `json:"chunk_id,omitempty"`
}

type TypeComposer struct {
	TypeName string `json:"type_name"`
	File     string `json:"file"`
	Line     int    `json:"line"`
}

type TypeFlowResult struct {
	TypeName      string         `json:"type_name"`
	Definition    TypeDefLoc     `json:"definition"`
	ForwardTrace  []TypeUsage    `json:"forward_trace"`
	BackwardTrace []TypeComposer `json:"backward_trace"`
}

func (e *Engine) handleTypeProvenance(q Query) (*Response, error) {
	node := e.flowIndex.GetTypeNode(q.Text)
	if node == nil {
		return &Response{
			Query: q.Text,
			Type:  "type_flow",
			Results: []Result{
				{Content: fmt.Sprintf("type %q not found in flow index", q.Text)},
			},
		}, nil
	}

	result := TypeFlowResult{
		TypeName: node.TypeName,
		Definition: TypeDefLoc{
			File: node.DefFile,
			Line: node.DefLine,
		},
	}

	for _, fn := range node.UsedAsParam {
		result.ForwardTrace = append(result.ForwardTrace, TypeUsage{
			Kind:   "param",
			FuncID: fn,
		})
	}
	for _, fn := range node.UsedAsReturn {
		result.ForwardTrace = append(result.ForwardTrace, TypeUsage{
			Kind:   "return",
			FuncID: fn,
		})
	}
	for _, fn := range node.UsedAsField {
		result.ForwardTrace = append(result.ForwardTrace, TypeUsage{
			Kind:   "field",
			FuncID: fn,
		})
	}

	for _, parent := range node.UsedAsField {
		parentNode := e.flowIndex.GetTypeNode(parent)
		if parentNode == nil {
			result.BackwardTrace = append(result.BackwardTrace, TypeComposer{
				TypeName: parent,
			})
			continue
		}
		result.BackwardTrace = append(result.BackwardTrace, TypeComposer{
			TypeName: parentNode.TypeName,
			File:     parentNode.DefFile,
			Line:     parentNode.DefLine,
		})
	}

	resp := &Response{
		Query: q.Text,
		Type:  "type_flow",
		Results: []Result{
			{Content: fmt.Sprintf("%+v", result)},
		},
	}
	return resp, nil
}
