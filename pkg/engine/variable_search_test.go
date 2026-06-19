package engine

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
)

func variableSearchTestEngine(t *testing.T) *Engine {
	t.Helper()
	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "foo", File: "main.go", Line: 1})

	fi := dataflow.NewFlowIndex()
	fg := &dataflow.FlowGraph{
		Variables: map[string]*dataflow.Variable{
			"userName":  {Name: "userName", Type: "string", Scope: dataflow.ScopeLocal, Pkg: "main", File: "main.go", DefLine: 2},
			"UserCount": {Name: "UserCount", Type: "int", Scope: dataflow.ScopeGlobal, Pkg: "main", File: "stats.go", DefLine: 1},
			"temp":      {Name: "temp", Type: "float64", Scope: dataflow.ScopeLocal, Pkg: "main", File: "calc.go", DefLine: 3},
			"config":    {Name: "config", Type: "Config", Scope: dataflow.ScopeParam, Pkg: "main", File: "server.go", DefLine: 5},
		},
		Defs:      make(map[string]*dataflow.VarDef),
		Uses:      make(map[string]*dataflow.VarUse),
		DefUseMap: make(map[string]*dataflow.DefUseChain),
		TypeNodes: make(map[string]*dataflow.TypeFlowNode),
	}
	fi.BuildFromFlowGraph(fg)

	return &Engine{
		flowIndex: fi,
		tokenizer: indexer.NewTokenizer(),
		graph:     graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph: cg,
	}
}

func TestVariableSearchExactMatch(t *testing.T) {
	eng := variableSearchTestEngine(t)
	resp, err := eng.handleVariableSearch(Query{Text: "userName"})
	if err != nil {
		t.Fatalf("handleVariableSearch: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}

func TestVariableSearchFuzzyMatch(t *testing.T) {
	eng := variableSearchTestEngine(t)
	resp, err := eng.handleVariableSearch(Query{Text: "user"})
	if err != nil {
		t.Fatalf("handleVariableSearch: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected fuzzy results")
	}
}

func TestVariableSearchByType(t *testing.T) {
	eng := variableSearchTestEngine(t)
	resp, err := eng.handleVariableSearch(Query{Text: "Config"})
	if err != nil {
		t.Fatalf("handleVariableSearch: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results for type search")
	}
}

func TestVariableSearchWithFileFilter(t *testing.T) {
	eng := variableSearchTestEngine(t)
	resp, err := eng.handleVariableSearch(Query{Text: "userName", File: "main.go"})
	if err != nil {
		t.Fatalf("handleVariableSearch: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results with file filter")
	}
}

func TestVariableSearchExportedBoost(t *testing.T) {
	eng := variableSearchTestEngine(t)
	resp, err := eng.handleVariableSearch(Query{Text: "Count"})
	if err != nil {
		t.Fatalf("handleVariableSearch: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}
