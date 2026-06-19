package dataflow_test

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

func newExtractor() *dataflow.DefUseExtractor {
	return &dataflow.DefUseExtractor{}
}

func countDefs(chains []*dataflow.DefUseChain) int {
	return len(chains)
}

func countUses(chains []*dataflow.DefUseChain) int {
	n := 0
	for _, c := range chains {
		n += len(c.Uses)
	}
	return n
}

func findUse(chains []*dataflow.DefUseChain, varName string, kind dataflow.UseKind) bool {
	for _, c := range chains {
		if c.Def.Variable != varName {
			continue
		}
		for _, u := range c.Uses {
			if u.Kind == kind {
				return true
			}
		}
	}
	return false
}

func findDef(chains []*dataflow.DefUseChain, varName string) *dataflow.DefUseChain {
	for _, c := range chains {
		if c.Def.Variable == varName {
			return c
		}
	}
	return nil
}

func TestExtractSimpleVar(t *testing.T) {
	src := `package main
func foo() {
	x := 42
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 1 {
		t.Errorf("expected 1 def, got %d", n)
	}
	if n := countUses(chains); n != 0 {
		t.Errorf("expected 0 uses, got %d", n)
	}
	if findDef(chains, "x") == nil {
		t.Errorf("expected def for x")
	}
}

func TestExtractDefAndUse(t *testing.T) {
	src := `package main
import "fmt"
func foo() {
	x := 42
	fmt.Println(x)
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 1 {
		t.Errorf("expected 1 def, got %d", n)
	}
	if n := countUses(chains); n != 1 {
		t.Errorf("expected 1 use, got %d", n)
	}
	if !findUse(chains, "x", dataflow.UseCallArg) {
		t.Errorf("expected use (call arg) for x")
	}
}

func TestExtractParam(t *testing.T) {
	src := `package main
import "fmt"
func foo(x int) {
	fmt.Println(x)
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 1 {
		t.Errorf("expected 1 def (param), got %d", n)
	}
	if n := countUses(chains); n != 1 {
		t.Errorf("expected 1 use, got %d", n)
	}

	chain := findDef(chains, "x")
	if chain == nil {
		t.Fatalf("expected def for x")
	}
	if chain.Def.Variable != "x" {
		t.Errorf("expected def variable x, got %s", chain.Def.Variable)
	}
}

func TestExtractReturn(t *testing.T) {
	src := `package main
func foo() int {
	x := 42
	return x
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 1 {
		t.Errorf("expected 1 def, got %d", n)
	}
	if n := countUses(chains); n != 1 {
		t.Errorf("expected 1 use, got %d", n)
	}
	if !findUse(chains, "x", dataflow.UseReturn) {
		t.Errorf("expected use (return) for x")
	}
}

func TestExtractCallArg(t *testing.T) {
	src := `package main
func bar(x int) {}
func foo() {
	y := 1
	bar(y)
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 2 {
		t.Errorf("expected 2 defs (x param, y local), got %d", n)
	}

	yChain := findDef(chains, "y")
	if yChain == nil {
		t.Fatalf("expected def for y")
	}
	hasCallArg := false
	for _, u := range yChain.Uses {
		if u.Kind == dataflow.UseCallArg {
			hasCallArg = true
			break
		}
	}
	if !hasCallArg {
		t.Errorf("expected use (call arg) for y")
	}
}

func TestExtractScope(t *testing.T) {
	src := `package main
import "fmt"
func foo() {
	x := 1
	{
		x := 2
		fmt.Println(x)
	}
	fmt.Println(x)
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 2 {
		t.Errorf("expected 2 defs (shadowing), got %d", n)
	}

	outerChain := findDef(chains, "x")
	if outerChain == nil {
		t.Fatalf("expected def for x (outer)")
	}
	if len(outerChain.Uses) != 1 {
		t.Errorf("expected 1 use for outer x, got %d uses: %+v", len(outerChain.Uses), outerChain.Uses)
	}

	innerChain := findDef(chains, "x")
	if innerChain == nil {
		t.Fatalf("expected def for x (inner)")
	}
}

func TestExtractRange(t *testing.T) {
	src := `package main
import "fmt"
func foo() {
	m := map[string]int{"a": 1}
	for k, v := range m {
		fmt.Println(k, v)
	}
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	kChain := findDef(chains, "k")
	if kChain == nil {
		t.Errorf("expected def for k")
	}
	vChain := findDef(chains, "v")
	if vChain == nil {
		t.Errorf("expected def for v")
	}
}

func TestExtractMultipleFiles(t *testing.T) {
	srcA := `package main
type T struct {}
func (t *T) DoSomething() {}
func NewT() *T {
	return &T{}
}`
	srcB := `package main
func Bar() {
	t := NewT()
	t.DoSomething()
}`

	fset := token.NewFileSet()
	fA, err := parser.ParseFile(fset, "a.go", srcA, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse a.go: %v", err)
	}
	fB, err := parser.ParseFile(fset, "b.go", srcB, parser.ParseComments)
	if err != nil {
		t.Fatalf("parse b.go: %v", err)
	}

	e := newExtractor()
	chainsA, err := e.ExtractDefUse(fA, fset, "a.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse a.go: %v", err)
	}
	chainsB, err := e.ExtractDefUse(fB, fset, "b.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUse b.go: %v", err)
	}

	_ = chainsA

	tChain := findDef(chainsB, "t")
	if tChain == nil {
		t.Fatalf("expected def for t in b.go")
	}

	if len(tChain.Uses) == 0 {
		t.Errorf("expected at least 1 use for t, got 0")
	}
}

func TestExtractWrite(t *testing.T) {
	src := `package main
func foo() {
	x := 1
	x = 2
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if n := countDefs(chains); n != 1 {
		t.Errorf("expected 1 def, got %d", n)
	}
	if !findUse(chains, "x", dataflow.UseWrite) {
		t.Errorf("expected use (write) for x")
	}
}

func TestExtractGlobalVar(t *testing.T) {
	src := `package main
var cfg = "default"
func foo() {
	_ = cfg
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	cfgChain := findDef(chains, "cfg")
	if cfgChain == nil {
		t.Errorf("expected def for cfg (global)")
	}
}

func TestExtractIfStmt(t *testing.T) {
	src := `package main
func foo(x int) {
	if x > 0 {
		y := 1
		_ = y
	}
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	xChain := findDef(chains, "x")
	if xChain == nil {
		t.Fatalf("expected def for x (param)")
	}
	hasRead := false
	for _, u := range xChain.Uses {
		if u.Kind == dataflow.UseRead {
			hasRead = true
			break
		}
	}
	if !hasRead {
		t.Errorf("expected use (read) for x in condition")
	}

	yChain := findDef(chains, "y")
	if yChain == nil {
		t.Errorf("expected def for y inside if body")
	}
}

func TestExtractForStmt(t *testing.T) {
	src := `package main
func foo() {
	for i := 0; i < 10; i++ {
		_ = i
	}
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	iChain := findDef(chains, "i")
	if iChain == nil {
		t.Fatalf("expected def for i")
	}
	if len(iChain.Uses) == 0 {
		t.Errorf("expected uses for i (condition, inc, body)")
	}
}

func TestExtractIncDec(t *testing.T) {
	src := `package main
func foo() {
	x := 1
	x++
}`
	chains, err := dataflow.ExtractDefUseFromSource(src, "main.go", "main")
	if err != nil {
		t.Fatalf("ExtractDefUseFromSource: %v", err)
	}

	if !findUse(chains, "x", dataflow.UseWrite) {
		t.Errorf("expected use (write) for x++")
	}
}
