package dataflow_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

type mockCallGraph struct{}

func (m *mockCallGraph) HasNode(id string) bool {
	return id == "main.NewUser" || id == "main.process"
}

func TestFlowGraphIntraProcedural(t *testing.T) {
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

	if fg == nil {
		t.Fatal("expected non-nil FlowGraph")
	}
	if len(fg.Defs) == 0 {
		t.Errorf("expected at least 1 def")
	}
	if len(fg.Edges) == 0 {
		t.Errorf("expected at least 1 edge")
	}
}

func TestFlowGraphReturnAssign(t *testing.T) {
	src := `package main
func NewUser() *User { return &User{} }
func Bar() {
	u := NewUser()
	_ = u
}`
	fg, err := dataflow.BuildFlowFromSource(src, "main.go", "main", &mockCallGraph{})
	if err != nil {
		t.Fatalf("BuildFlowFromSource: %v", err)
	}

	if len(fg.Defs) == 0 {
		t.Errorf("expected defs")
	}
	if fg.DefUseMap == nil {
		t.Errorf("expected def use map")
	}
	if fg.TypeNodes == nil {
		t.Errorf("expected type nodes from tracker")
	}
}

func TestFlowGraphInterProcedural(t *testing.T) {
	src := `package main
func Bar() {
	u := NewUser()
	process(u)
}`
	fg, err := dataflow.BuildFlowFromSource(src, "main.go", "main", &mockCallGraph{})
	if err != nil {
		t.Fatalf("BuildFlowFromSource: %v", err)
	}

	if fg == nil {
		t.Fatal("expected non-nil FlowGraph")
	}
}

func TestFlowGraphNoCallGraph(t *testing.T) {
	src := `package main
func foo() {
	x := 1
	_ = x
}`
	fg, err := dataflow.BuildFlowFromSource(src, "main.go", "main", nil)
	if err != nil {
		t.Fatalf("BuildFlowFromSource with nil cg: %v", err)
	}
	if fg == nil {
		t.Fatal("expected non-nil FlowGraph")
	}
}

func TestFlowGraphBuilderReuse(t *testing.T) {
	builder := dataflow.NewFlowGraphBuilder(&mockCallGraph{})
	if builder == nil {
		t.Fatal("expected non-nil builder")
	}
}
