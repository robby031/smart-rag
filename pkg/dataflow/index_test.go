package dataflow_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

func buildTestFlowGraph(t *testing.T) *dataflow.FlowGraph {
	t.Helper()
	src := `package main
func foo() {
	x := 1
	y := x + 1
	_ = y
}`
	fg, err := dataflow.BuildFlowFromSource(src, "main.go", "main", &mockCallGraph{})
	if err != nil {
		t.Fatalf("BuildFlowFromSource: %v", err)
	}
	return fg
}

func TestIndexBuildFromFlowGraph(t *testing.T) {
	fg := buildTestFlowGraph(t)
	idx := dataflow.NewFlowIndex()
	idx.BuildFromFlowGraph(fg)

	if idx == nil {
		t.Fatal("expected non-nil index")
	}
}

func TestIndexByVariableName(t *testing.T) {
	fg := buildTestFlowGraph(t)
	idx := dataflow.NewFlowIndex()
	idx.BuildFromFlowGraph(fg)

	vars := idx.ByVariableName("x")
	if len(vars) == 0 {
		t.Errorf("expected at least 1 variable named x")
	}
}

func TestIndexByFile(t *testing.T) {
	fg := buildTestFlowGraph(t)
	idx := dataflow.NewFlowIndex()
	idx.BuildFromFlowGraph(fg)

	vars := idx.ByFile("main.go")
	if len(vars) == 0 {
		t.Errorf("expected at least 1 variable in main.go")
	}
}

func TestIndexGetChain(t *testing.T) {
	fg := buildTestFlowGraph(t)
	idx := dataflow.NewFlowIndex()
	idx.BuildFromFlowGraph(fg)

	for defID := range fg.DefUseMap {
		chain := idx.GetChain(defID)
		if chain == nil {
			t.Errorf("expected chain for def %s", defID)
		}
	}
}

func TestIndexSearchVariable(t *testing.T) {
	fg := buildTestFlowGraph(t)
	idx := dataflow.NewFlowIndex()
	idx.BuildFromFlowGraph(fg)

	results := idx.SearchVariable("x")
	if len(results) == 0 {
		t.Errorf("expected at least 1 result for search 'x'")
	}
}

func TestIndexEmpty(t *testing.T) {
	idx := dataflow.NewFlowIndex()
	if idx == nil {
		t.Fatal("expected non-nil index")
	}
	if vars := idx.ByVariableName("nonexistent"); len(vars) != 0 {
		t.Errorf("expected empty result, got %d", len(vars))
	}
}
