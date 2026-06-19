package engine

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
)

func dynamicFlowTestEngine(t *testing.T) *Engine {
	t.Helper()
	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "process", File: "main.go", Line: 1})

	fi := dataflow.NewFlowIndex()
	fg := &dataflow.FlowGraph{
		Variables: make(map[string]*dataflow.Variable),
		Defs:      make(map[string]*dataflow.VarDef),
		Uses:      make(map[string]*dataflow.VarUse),
		DefUseMap: make(map[string]*dataflow.DefUseChain),
		TypeNodes: make(map[string]*dataflow.TypeFlowNode),
	}
	fi.BuildFromFlowGraph(fg)

	return &Engine{
		flowIndex: fi,
		flowStore: nil,
		graph:     graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph: cg,
	}
}

func TestDynamicQueryNoStore(t *testing.T) {
	eng := dynamicFlowTestEngine(t)
	resp, err := eng.handleDynamicFlow(Query{Text: "main.process"})
	if err != nil {
		t.Fatalf("handleDynamicFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected response")
	}
}

func TestDynamicQueryEmptyText(t *testing.T) {
	eng := dynamicFlowTestEngine(t)
	resp, err := eng.handleDynamicFlow(Query{Text: ""})
	if err != nil {
		t.Fatalf("handleDynamicFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected response")
	}
}

func TestDynamicQueryByFuncID(t *testing.T) {
	eng := dynamicFlowTestEngine(t)
	resp, err := eng.handleDynamicFlow(Query{Text: "main.process"})
	if err != nil {
		t.Fatalf("handleDynamicFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}

func TestDynamicQueryByVarName(t *testing.T) {
	eng := dynamicFlowTestEngine(t)
	resp, err := eng.handleDynamicFlow(Query{Text: "x"})
	if err != nil {
		t.Fatalf("handleDynamicFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}

func TestTraceEventView(t *testing.T) {
	view := TraceEventView{
		EventType: "assign",
		VarName:   "x",
		Value:     "42",
		File:      "main.go",
		Line:      5,
	}
	if view.EventType != "assign" {
		t.Errorf("expected assign, got %s", view.EventType)
	}
	if view.VarName != "x" {
		t.Errorf("expected var x, got %s", view.VarName)
	}
	if view.Value != "42" {
		t.Errorf("expected value 42, got %s", view.Value)
	}
	if view.File != "main.go" {
		t.Errorf("expected main.go, got %s", view.File)
	}
	if view.Line != 5 {
		t.Errorf("expected line 5, got %d", view.Line)
	}
}
