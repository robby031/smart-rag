package dataflow_test

import (
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

func TestJSFunction(t *testing.T) {
	src := `function add(x, y) { return x + y }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected at least 1 chain")
	}
}

func TestJSConstLet(t *testing.T) {
	src := `function foo() { const x = 1; let y = 2; y = 3 }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected chains")
	}
}

func TestJSArrowFunction(t *testing.T) {
	src := `const add = (a, b) => a + b`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected chains")
	}
}

func TestJSClass(t *testing.T) {
	src := `class User { constructor(name) { this.name = name } getName() { return this.name } }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected chains for class methods")
	}
}

func TestJSScope(t *testing.T) {
	src := `function foo() { let x = 1; if (true) { let x = 2; console.log(x) }; console.log(x) }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected chains")
	}
}

func TestJSCallArg(t *testing.T) {
	src := `function foo() { bar(42) }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	_ = chains
}

func TestJSReturn(t *testing.T) {
	src := `function foo(x) { return x }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected chains")
	}
}

func TestJSFlowGraph(t *testing.T) {
	src := `function add(x, y) { return x + y }`
	b := dataflow.NewJSFlowGraphBuilder()
	fg, err := b.BuildFromSource(src, "test.js", "main")
	if err != nil {
		t.Fatalf("BuildFromSource: %v", err)
	}
	if fg == nil {
		t.Fatal("expected flow graph")
	}
	if len(fg.Defs) == 0 {
		t.Errorf("expected defs")
	}
}

func TestJSTypeFlow(t *testing.T) {
	src := []byte(`interface User { name: string; age: number }`)
	tracker := dataflow.NewJSTypeFlowTracker()
	if err := tracker.ExtractTypes(src, "test.ts", "main"); err != nil {
		t.Fatalf("ExtractTypes: %v", err)
	}
	nodes := tracker.GetAllNodes()
	if len(nodes) == 0 {
		t.Error("expected type nodes")
	}
}

func TestJSTypeFlowJS(t *testing.T) {
	src := []byte(`function foo(x) { return x }`)
	tracker := dataflow.NewJSTypeFlowTracker()
	if err := tracker.ExtractTypes(src, "test.js", "main"); err != nil {
		t.Fatalf("ExtractTypes should not error for JS: %v", err)
	}
}

func TestJSForLoopVar(t *testing.T) {
	src := `function foo() { for (let i = 0; i < 10; i++) { console.log(i) } }`
	e := dataflow.NewJSDefUseExtractor()
	chains, err := e.ExtractDefUse(src, "test.js", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse: %v", err)
	}
	if len(chains) == 0 {
		t.Fatal("expected chains for for loop")
	}
}
