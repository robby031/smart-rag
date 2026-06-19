package engine

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
)

func traceTestEngine(t *testing.T) *Engine {
	t.Helper()
	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "foo", File: "main.go", Line: 1})
	cg.AddNode(&graph.Node{Pkg: "main", Name: "bar", File: "main.go", Line: 5})

	fi := dataflow.NewFlowIndex()
	fg := &dataflow.FlowGraph{
		Variables: map[string]*dataflow.Variable{
			"x": {Name: "x", Type: "int", Scope: dataflow.ScopeLocal, Pkg: "main", File: "main.go", DefLine: 2},
			"y": {Name: "y", Type: "int", Scope: dataflow.ScopeLocal, Pkg: "main", File: "main.go", DefLine: 3},
		},
		Defs: map[string]*dataflow.VarDef{
			"main.main.go:2:x": {ID: "main.main.go:2:x", Variable: "x", Pkg: "main", File: "main.go", StartLine: 2},
			"main.main.go:3:y": {ID: "main.main.go:3:y", Variable: "y", Pkg: "main", File: "main.go", StartLine: 3},
		},
		Uses: map[string]*dataflow.VarUse{
			"main.main.go:4:x:0": {ID: "main.main.go:4:x:0", Variable: "x", File: "main.go", Line: 4, Kind: dataflow.UseRead, FuncID: "main.foo"},
		},
		DefUseMap: map[string]*dataflow.DefUseChain{
			"main.main.go:2:x": {Def: dataflow.VarDef{ID: "main.main.go:2:x", Variable: "x", Pkg: "main", File: "main.go", StartLine: 2}},
			"main.main.go:3:y": {Def: dataflow.VarDef{ID: "main.main.go:3:y", Variable: "y", Pkg: "main", File: "main.go", StartLine: 3}},
		},
		TypeNodes: make(map[string]*dataflow.TypeFlowNode),
	}
	fi.BuildFromFlowGraph(fg)

	return &Engine{
		flowIndex: fi,
		graph:     graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph: cg,
	}
}

func TestTraceVariableNotFound(t *testing.T) {
	eng := traceTestEngine(t)
	resp, err := eng.handleTraceVariable(Query{Text: "nonexistent"})
	if err != nil {
		t.Fatalf("handleTraceVariable: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected response")
	}
}

func TestTraceVariableLocal(t *testing.T) {
	eng := traceTestEngine(t)
	resp, err := eng.handleTraceVariable(Query{Text: "x"})
	if err != nil {
		t.Fatalf("handleTraceVariable: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected trace results")
	}
}

func TestTraceVariableWithFileFilter(t *testing.T) {
	eng := traceTestEngine(t)
	resp, err := eng.handleTraceVariable(Query{Text: "x", File: "main.go"})
	if err != nil {
		t.Fatalf("handleTraceVariable: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected trace results with file filter")
	}
}

func TestTraceBFS(t *testing.T) {
	eng := traceTestEngine(t)
	steps, provenance := eng.traceBFS("x", 3)
	if len(steps) == 0 {
		t.Error("expected at least 1 trace step")
	}
	_ = provenance
}

func TestUseKindString(t *testing.T) {
	tests := []struct {
		kind dataflow.UseKind
		want string
	}{
		{dataflow.UseRead, "read"},
		{dataflow.UseWrite, "write"},
		{dataflow.UseCallArg, "call_arg"},
		{dataflow.UseReturn, "return"},
		{dataflow.UseKind(99), "unknown"},
	}
	for _, tt := range tests {
		got := useKindString(tt.kind)
		if got != tt.want {
			t.Errorf("useKindString(%d) = %s, want %s", tt.kind, got, tt.want)
		}
	}
}
