package engine

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/graph"
)

func typeProvenanceTestEngine(t *testing.T) *Engine {
	t.Helper()
	cg := graph.NewCallGraph()
	cg.AddNode(&graph.Node{Pkg: "main", Name: "CreateUser", File: "main.go", Line: 5})
	cg.AddNode(&graph.Node{Pkg: "main", Name: "GetUser", File: "main.go", Line: 10})

	fi := dataflow.NewFlowIndex()
	fg := &dataflow.FlowGraph{
		Variables: make(map[string]*dataflow.Variable),
		Defs:      make(map[string]*dataflow.VarDef),
		Uses:      make(map[string]*dataflow.VarUse),
		DefUseMap: make(map[string]*dataflow.DefUseChain),
		TypeNodes: map[string]*dataflow.TypeFlowNode{
			"User": {
				TypeName:     "User",
				DefFile:      "model.go",
				DefLine:      1,
				UsedAsParam:  []string{"main.CreateUser", "main.GetUser"},
				UsedAsReturn: []string{"main.FindUser"},
				UsedAsField:  []string{"Admin", "Profile"},
			},
		},
	}
	fi.BuildFromFlowGraph(fg)

	return &Engine{
		flowIndex: fi,
		graph:     graph.NewGraph(cg, graph.NewImportGraph()),
		callGraph: cg,
	}
}

func TestTypeProvenanceNotFound(t *testing.T) {
	eng := typeProvenanceTestEngine(t)
	resp, err := eng.handleTypeProvenance(Query{Text: "Nonexistent"})
	if err != nil {
		t.Fatalf("handleTypeProvenance: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected response")
	}
}

func TestTypeProvenanceForwardTrace(t *testing.T) {
	eng := typeProvenanceTestEngine(t)
	resp, err := eng.handleTypeProvenance(Query{Text: "User"})
	if err != nil {
		t.Fatalf("handleTypeProvenance: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}

func TestTypeProvenanceBackwardTrace(t *testing.T) {
	eng := typeProvenanceTestEngine(t)
	resp, err := eng.handleTypeProvenance(Query{Text: "User"})
	if err != nil {
		t.Fatalf("handleTypeProvenance: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected results")
	}
}
