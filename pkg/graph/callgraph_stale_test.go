package graph_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
)

func TestCallGraphNoGhostNodes(t *testing.T) {
	cg := graph.NewCallGraph()

	filePath := "pkg/engine/engine.go"
	pkg := "engine"

	firstSrc := `package engine
func OldFunc() { Helper() }
func Helper() {}
`
	if err := cg.ParseFile(filePath, firstSrc, pkg); err != nil {
		t.Fatalf("first ParseFile: %v", err)
	}

	if _, ok := cg.Nodes["engine.OldFunc"]; !ok {
		t.Fatal("expected engine.OldFunc after first pass")
	}
	if _, ok := cg.Nodes["engine.Helper"]; !ok {
		t.Fatal("expected engine.Helper after first pass")
	}

	cg.DeleteByFile(filePath)

	secondSrc := `package engine
func NewFunc() { Helper() }
func Helper() {}
`
	if err := cg.ParseFile(filePath, secondSrc, pkg); err != nil {
		t.Fatalf("second ParseFile: %v", err)
	}

	// OldFunc must be gone.
	if _, ok := cg.Nodes["engine.OldFunc"]; ok {
		t.Errorf("ghost node engine.OldFunc still present after re-index")
	}
	// NewFunc and Helper must be present.
	if _, ok := cg.Nodes["engine.NewFunc"]; !ok {
		t.Errorf("engine.NewFunc missing after re-index")
	}
	if _, ok := cg.Nodes["engine.Helper"]; !ok {
		t.Errorf("engine.Helper missing after re-index")
	}
}

func TestCallGraphDeleteByFileEdges(t *testing.T) {
	cg := graph.NewCallGraph()

	filePath := "pkg/foo/foo.go"
	pkg := "foo"

	// First pass: A calls B and C.
	firstSrc := `package foo
func A() { B(); C() }
func B() {}
func C() {}
`
	if err := cg.ParseFile(filePath, firstSrc, pkg); err != nil {
		t.Fatalf("first ParseFile: %v", err)
	}
	if len(cg.OutEdges["foo.A"]) == 0 {
		t.Fatal("expected out-edges for foo.A after first pass")
	}

	// Second pass: A is removed, only B and D remain.
	cg.DeleteByFile(filePath)

	secondSrc := `package foo
func B() {}
func D() {}
`
	if err := cg.ParseFile(filePath, secondSrc, pkg); err != nil {
		t.Fatalf("second ParseFile: %v", err)
	}

	// foo.A must have no out-edges.
	if len(cg.OutEdges["foo.A"]) > 0 {
		t.Errorf("stale out-edges for ghost node foo.A: %v", cg.OutEdges["foo.A"])
	}
	// foo.C must be gone (it was only defined in this file).
	if _, ok := cg.Nodes["foo.C"]; ok {
		t.Errorf("ghost node foo.C still present after re-index")
	}
	// foo.B and foo.D must be present.
	if _, ok := cg.Nodes["foo.B"]; !ok {
		t.Errorf("foo.B missing after re-index")
	}
	if _, ok := cg.Nodes["foo.D"]; !ok {
		t.Errorf("foo.D missing after re-index")
	}
}

func TestCallGraphDeleteByFileIsolation(t *testing.T) {
	cg := graph.NewCallGraph()

	file1 := "pkg/a/a.go"
	file2 := "pkg/b/b.go"

	src1 := `package a
func FuncA() {}
`
	src2 := `package b
func FuncB() {}
`
	if err := cg.ParseFile(file1, src1, "a"); err != nil {
		t.Fatalf("ParseFile a: %v", err)
	}
	if err := cg.ParseFile(file2, src2, "b"); err != nil {
		t.Fatalf("ParseFile b: %v", err)
	}

	// Delete only file1.
	cg.DeleteByFile(file1)

	if _, ok := cg.Nodes["a.FuncA"]; ok {
		t.Errorf("a.FuncA should have been deleted")
	}
	if _, ok := cg.Nodes["b.FuncB"]; !ok {
		t.Errorf("b.FuncB in unrelated file must not be deleted")
	}
}
