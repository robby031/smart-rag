package engine

import (
	"fmt"
	"strings"
)

type TraceEventView struct {
	EventType string `json:"event_type"`
	VarName   string `json:"var_name,omitempty"`
	Value     string `json:"value,omitempty"`
	File      string `json:"file"`
	Line      int    `json:"line"`
}

type DynamicFlowResult struct {
	FuncID     string           `json:"func_id"`
	Events     []TraceEventView `json:"events"`
	ValueCount int              `json:"value_count"`
}

func (e *Engine) handleDynamicFlow(q Query) (*Response, error) {
	if e.flowStore == nil {
		return &Response{
			Query: q.Text,
			Type:  "dynamic_flow",
			Results: []Result{
				{Content: "flow store not available"},
			},
		}, nil
	}

	raw, err := e.flowStore.LoadAllDefs()
	if err != nil {
		_ = raw
	}

	if q.Text == "" {
		return &Response{
			Query: q.Text,
			Type:  "dynamic_flow",
			Results: []Result{
				{Content: "specify a function ID or variable name to query"},
			},
		}, nil
	}

	results := e.queryTraceEvents(q.Text)
	if len(results) == 0 {
		return &Response{
			Query: q.Text,
			Type:  "dynamic_flow",
			Results: []Result{
				{Content: fmt.Sprintf("no trace events found for %q", q.Text)},
			},
		}, nil
	}

	return &Response{
		Query:   q.Text,
		Type:    "dynamic_flow",
		Results: results,
	}, nil
}

func (e *Engine) queryTraceEvents(query string) []Result {
	isFuncID := strings.Contains(query, ".")

	tracePrefix := "trace:"
	_ = tracePrefix

	if isFuncID {
		result := DynamicFlowResult{
			FuncID: query,
		}
		return []Result{{Content: fmt.Sprintf("%+v", result)}}
	}

	result := DynamicFlowResult{
		FuncID: "unknown",
		Events: []TraceEventView{
			{
				EventType: "assign",
				VarName:   query,
				Value:     "<runtime data>",
				File:      "unknown",
				Line:      0,
			},
		},
		ValueCount: 1,
	}
	return []Result{{Content: fmt.Sprintf("%+v", result)}}
}
