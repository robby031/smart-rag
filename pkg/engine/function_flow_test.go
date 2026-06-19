package engine

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
)

func functionFlowTestEngine(t *testing.T) *Engine {
	t.Helper()
	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "process", File: "main.go", Line: 1, Recv: ""})
	cg.AddNode(&graph.Node{Pkg: "main", Name: "foo", File: "main.go", Line: 5})

	cg.AddEdge("main.process", "main.foo", 0, "")

	fi := dataflow.NewFlowIndex()
	fg := &dataflow.FlowGraph{
		Variables: map[string]*dataflow.Variable{
			"name": {Name: "name", Type: "string", Scope: dataflow.ScopeParam, Pkg: "main", File: "main.go", DefLine: 1},
			"age":  {Name: "age", Type: "int", Scope: dataflow.ScopeParam, Pkg: "main", File: "main.go", DefLine: 1},
			"x":    {Name: "x", Type: "int", Scope: dataflow.ScopeLocal, Pkg: "main", File: "main.go", DefLine: 2},
			"cfg":  {Name: "cfg", Type: "string", Scope: dataflow.ScopeGlobal, Pkg: "main", File: "config.go", DefLine: 1},
		},
		Defs:      make(map[string]*dataflow.VarDef),
		Uses:      make(map[string]*dataflow.VarUse),
		DefUseMap: make(map[string]*dataflow.DefUseChain),
		TypeNodes: make(map[string]*dataflow.TypeFlowNode),
	}
	fi.BuildFromFlowGraph(fg)

	return &Engine{
		flowIndex: fi,
		graph:     graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph: cg,
	}
}

func TestFunctionFlowNotFound(t *testing.T) {
	eng := functionFlowTestEngine(t)
	resp, err := eng.handleFunctionFlow(Query{Text: "nonexistent"})
	if err != nil {
		t.Fatalf("handleFunctionFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected response")
	}
}

func TestFunctionFlowWithParams(t *testing.T) {
	eng := functionFlowTestEngine(t)
	resp, err := eng.handleFunctionFlow(Query{Text: "process"})
	if err != nil {
		t.Fatalf("handleFunctionFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}

func TestFunctionFlowWithInternals(t *testing.T) {
	eng := functionFlowTestEngine(t)
	resp, err := eng.handleFunctionFlow(Query{Text: "foo"})
	if err != nil {
		t.Fatalf("handleFunctionFlow: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}
