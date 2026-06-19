package dataflow_test

import (
	"path/filepath"
	"testing"

	"github.com/robby031/smart-rag/pkg/dataflow"
	"github.com/robby031/smart-rag/pkg/storage"
)

func TestFlowStore_VariableRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	v := &dataflow.Variable{
		Name:     "counter",
		Type:     "int",
		Scope:    dataflow.ScopeLocal,
		Pkg:      "main",
		File:     "main.go",
		DefLine:  10,
		DefChunk: "main.go:10-12",
	}

	if err := fs.SaveVariable(v); err != nil {
		t.Fatalf("SaveVariable: %v", err)
	}

	got, err := fs.LoadVariable("main", "counter")
	if err != nil {
		t.Fatalf("LoadVariable: %v", err)
	}

	if got.Name != v.Name || got.Type != v.Type || got.Scope != v.Scope {
		t.Errorf("Variable mismatch: got %+v, want %+v", got, v)
	}
}

func TestFlowStore_DefUseChainRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	def := &dataflow.VarDef{
		ID:        "main.main.go:10:counter",
		Variable:  "counter",
		Pkg:       "main",
		File:      "main.go",
		StartLine: 10,
		EndLine:   12,
		ChunkID:   "main.go:10-12",
	}

	use := &dataflow.VarUse{
		ID:       "main.main.go:15:counter:read",
		Variable: "counter",
		File:     "main.go",
		Line:     15,
		Kind:     dataflow.UseRead,
		FuncID:   "main.main",
	}

	if err := fs.SaveDef(def); err != nil {
		t.Fatalf("SaveDef: %v", err)
	}
	if err := fs.SaveUse(use); err != nil {
		t.Fatalf("SaveUse: %v", err)
	}

	chain := &dataflow.DefUseChain{
		Def:  *def,
		Uses: []dataflow.VarUse{*use},
	}
	if err := fs.SaveChain(chain); err != nil {
		t.Fatalf("SaveChain: %v", err)
	}

	// Verify by loading all defs and uses
	defs, err := fs.LoadAllDefs()
	if err != nil {
		t.Fatalf("LoadAllDefs: %v", err)
	}
	if len(defs) != 1 {
		t.Errorf("expected 1 def, got %d", len(defs))
	}

	uses, err := fs.LoadAllUses()
	if err != nil {
		t.Fatalf("LoadAllUses: %v", err)
	}
	if len(uses) != 1 {
		t.Errorf("expected 1 use, got %d", len(uses))
	}
}

func TestFlowStore_TypeNodeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	node := &dataflow.TypeFlowNode{
		TypeName:     "User",
		DefFile:      "model.go",
		DefLine:      5,
		UsedAsParam:  []string{"main.CreateUser", "main.GetUser"},
		UsedAsReturn: []string{"main.FindUser"},
		UsedAsField:  []string{"Profile"},
	}

	if err := fs.SaveTypeNode(node); err != nil {
		t.Fatalf("SaveTypeNode: %v", err)
	}

	got, err := fs.LoadTypeNode("User")
	if err != nil {
		t.Fatalf("LoadTypeNode: %v", err)
	}

	if got.TypeName != node.TypeName || got.DefFile != node.DefFile {
		t.Errorf("TypeNode mismatch: got %+v, want %+v", got, node)
	}
}

func TestFlowStore_EdgeRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	edge := &dataflow.DataFlowEdge{
		From: "main.main.go:10:counter",
		To:   "main.main.go:15:counter:read",
		Kind: "def_use",
		File: "main.go",
		Line: 15,
	}

	if err := fs.SaveEdge(edge); err != nil {
		t.Fatalf("SaveEdge: %v", err)
	}

	edges, err := fs.LoadAllEdges()
	if err != nil {
		t.Fatalf("LoadAllEdges: %v", err)
	}
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	if edges[0].From != edge.From || edges[0].Kind != edge.Kind {
		t.Errorf("Edge mismatch: got %+v, want %+v", edges[0], edge)
	}
}

func TestFlowStore_FuncVariables(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	funcID := "main.processOrder"
	varNames := []string{"order", "total", "discount"}

	if err := fs.SaveFuncVariables(funcID, varNames); err != nil {
		t.Fatalf("SaveFuncVariables: %v", err)
	}

	got, err := fs.LoadFuncVariables(funcID)
	if err != nil {
		t.Fatalf("LoadFuncVariables: %v", err)
	}

	if len(got) != len(varNames) {
		t.Fatalf("expected %d names, got %d", len(varNames), len(got))
	}
	for i, name := range varNames {
		if got[i] != name {
			t.Errorf("name[%d]: got %s, want %s", i, got[i], name)
		}
	}
}

func TestFlowStore_DeleteByFile(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	// Simpan data untuk dua file berbeda
	fileA := "pkg/a/file.go"
	fileB := "pkg/b/file.go"

	vA := &dataflow.Variable{Name: "xA", Type: "int", Scope: dataflow.ScopeLocal, Pkg: "a", File: fileA, DefLine: 1}
	vB := &dataflow.Variable{Name: "xB", Type: "string", Scope: dataflow.ScopeLocal, Pkg: "b", File: fileB, DefLine: 1}

	if err := fs.SaveVariable(vA); err != nil {
		t.Fatalf("SaveVariable vA: %v", err)
	}
	if err := fs.SaveVariable(vB); err != nil {
		t.Fatalf("SaveVariable vB: %v", err)
	}

	defA := &dataflow.VarDef{ID: "a.pkg/a/file.go:1:xA", Variable: "xA", Pkg: "a", File: fileA, StartLine: 1, EndLine: 3}
	defB := &dataflow.VarDef{ID: "b.pkg/b/file.go:1:xB", Variable: "xB", Pkg: "b", File: fileB, StartLine: 1, EndLine: 3}
	if err := fs.SaveDef(defA); err != nil {
		t.Fatalf("SaveDef defA: %v", err)
	}
	if err := fs.SaveDef(defB); err != nil {
		t.Fatalf("SaveDef defB: %v", err)
	}

	// Hapus semua data untuk fileA
	if err := fs.DeleteByFile(fileA); err != nil {
		t.Fatalf("DeleteByFile: %v", err)
	}

	// Data fileA harus hilang
	_, err = fs.LoadVariable("a", "xA")
	if err == nil {
		t.Errorf("expected error loading deleted variable xA")
	}

	// Data fileB harus tetap ada
	gotB, err := fs.LoadVariable("b", "xB")
	if err != nil {
		t.Fatalf("LoadVariable xB after delete: %v", err)
	}
	if gotB.Name != "xB" {
		t.Errorf("expected xB, got %s", gotB.Name)
	}

	// Def fileA harus hilang
	defs, err := fs.LoadAllDefs()
	if err != nil {
		t.Fatalf("LoadAllDefs: %v", err)
	}
	if _, exists := defs["a.pkg/a/file.go:1:xA"]; exists {
		t.Errorf("defA should have been deleted")
	}
	if _, exists := defs["b.pkg/b/file.go:1:xB"]; !exists {
		t.Errorf("defB should still exist")
	}
}

func TestFlowStore_DeleteByFunc(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	funcID := "main.process"
	if err := fs.SaveFuncVariables(funcID, []string{"x", "y"}); err != nil {
		t.Fatalf("SaveFuncVariables: %v", err)
	}

	use := &dataflow.VarUse{
		ID:       "main.main.go:10:x:read",
		Variable: "x",
		File:     "main.go",
		Line:     10,
		Kind:     dataflow.UseRead,
		FuncID:   funcID,
	}
	if err := fs.SaveUse(use); err != nil {
		t.Fatalf("SaveUse: %v", err)
	}

	// Hapus data untuk fungsi
	if err := fs.DeleteByFunc(funcID); err != nil {
		t.Fatalf("DeleteByFunc: %v", err)
	}

	// Func variables harus hilang
	_, err = fs.LoadFuncVariables(funcID)
	if err == nil {
		t.Errorf("expected error after DeleteByFunc")
	}

	// Use dengan funcID yang sama harus hilang
	uses, err := fs.LoadAllUses()
	if err != nil {
		t.Fatalf("LoadAllUses: %v", err)
	}
	if len(uses) != 0 {
		t.Errorf("expected 0 uses after DeleteByFunc, got %d", len(uses))
	}
}

func TestFlowStore_MetaRoundTrip(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	meta := &dataflow.FlowMeta{
		VariableCount: 10,
		DefCount:      5,
		UseCount:      20,
		ChainCount:    5,
		TypeNodeCount: 3,
		EdgeCount:     15,
	}

	if err := fs.SaveMeta(meta); err != nil {
		t.Fatalf("SaveMeta: %v", err)
	}

	got, err := fs.LoadMeta()
	if err != nil {
		t.Fatalf("LoadMeta: %v", err)
	}

	if got.VariableCount != meta.VariableCount || got.DefCount != meta.DefCount {
		t.Errorf("Meta mismatch: got %+v, want %+v", got, meta)
	}
}

func TestFlowStore_LoadAllVariables(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	vars := []*dataflow.Variable{
		{Name: "a", Type: "int", Scope: dataflow.ScopeLocal, Pkg: "main", File: "main.go", DefLine: 1},
		{Name: "b", Type: "string", Scope: dataflow.ScopeParam, Pkg: "main", File: "main.go", DefLine: 5},
		{Name: "cfg", Type: "Config", Scope: dataflow.ScopeGlobal, Pkg: "config", File: "config.go", DefLine: 3},
	}

	for _, v := range vars {
		if err := fs.SaveVariable(v); err != nil {
			t.Fatalf("SaveVariable %s: %v", v.Name, err)
		}
	}

	loaded, err := fs.LoadAllVariables()
	if err != nil {
		t.Fatalf("LoadAllVariables: %v", err)
	}

	if len(loaded) != len(vars) {
		t.Errorf("expected %d variables, got %d", len(vars), len(loaded))
	}
}

func TestFlowStore_DeleteByFileEmpty(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	// DeleteByFile pada file yang tidak ada harus tidak error
	if err := fs.DeleteByFile("nonexistent.go"); err != nil {
		t.Errorf("DeleteByFile on nonexistent file: %v", err)
	}
}

func TestFlowStore_DeleteByFuncEmpty(t *testing.T) {
	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "flow_test"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer kv.Close()
	fs := storage.NewFlowStore(kv)

	// DeleteByFunc pada func yang tidak ada harus tidak error
	if err := fs.DeleteByFunc("nonexistent.func"); err != nil {
		t.Errorf("DeleteByFunc on nonexistent func: %v", err)
	}
}
