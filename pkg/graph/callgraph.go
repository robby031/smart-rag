package graph

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"

	"github.com/bagusdwiharianto/smart-rag/pkg/storage"
)

type Node struct {
	Pkg  string `json:"pkg"`
	Name string `json:"name"`
	Recv string `json:"recv,omitempty"`
	File string `json:"file"`
	Line int    `json:"line"`
}

func (n *Node) ID() string {
	recv := ""
	if n.Recv != "" {
		recv = "(" + n.Recv + ")."
	}
	return fmt.Sprintf("%s.%s%s", n.Pkg, recv, n.Name)
}

// CallGraph uses adjacency lists for O(1) neighbor lookups.
type CallGraph struct {
	Nodes    map[string]*Node
	OutEdges map[string]map[string]bool // caller -> set of callees
	InEdges  map[string]map[string]bool // callee -> set of callers
	EdgeMeta map[string]*edgeMeta       // "caller\x00callee" -> metadata
	Fset     *token.FileSet
	store    *storage.GraphStore
}

type edgeMeta struct {
	Line int
	File string
}

func NewCallGraph() *CallGraph {
	return &CallGraph{
		Nodes:    make(map[string]*Node),
		OutEdges: make(map[string]map[string]bool),
		InEdges:  make(map[string]map[string]bool),
		EdgeMeta: make(map[string]*edgeMeta),
		Fset:     token.NewFileSet(),
	}
}

func NewPersistentCallGraph(gs *storage.GraphStore) *CallGraph {
	cg := NewCallGraph()
	cg.store = gs
	if gs != nil {
		cg.load()
	}
	return cg
}

func (cg *CallGraph) AddNode(n *Node) {
	id := n.ID()
	if _, ok := cg.Nodes[id]; !ok {
		cg.Nodes[id] = n
		if cg.store != nil {
			cg.store.SaveNode(storageNode(n))
		}
	}
}

func (cg *CallGraph) AddEdge(caller, callee string, line int, file string) {
	if cg.OutEdges[caller] == nil {
		cg.OutEdges[caller] = make(map[string]bool)
	}
	if cg.InEdges[callee] == nil {
		cg.InEdges[callee] = make(map[string]bool)
	}

	if !cg.OutEdges[caller][callee] {
		cg.OutEdges[caller][callee] = true
		cg.InEdges[callee][caller] = true
		key := caller + "\x00" + callee
		cg.EdgeMeta[key] = &edgeMeta{Line: line, File: file}

		if cg.store != nil {
			cg.store.SaveEdge(storage.GraphEdge{
				Caller: caller,
				Callee: callee,
				Line:   line,
				File:   file,
			})
		}
	}
}

func (cg *CallGraph) Callees(nodeID string) []string {
	set := cg.OutEdges[nodeID]
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (cg *CallGraph) Callers(nodeID string) []string {
	set := cg.InEdges[nodeID]
	if len(set) == 0 {
		return nil
	}
	result := make([]string, 0, len(set))
	for id := range set {
		result = append(result, id)
	}
	sort.Strings(result)
	return result
}

func (cg *CallGraph) EdgeCount() int {
	return len(cg.EdgeMeta)
}

func (cg *CallGraph) ParseFile(filePath, src string, pkg string) error {
	f, err := parser.ParseFile(cg.Fset, filePath, src, parser.ParseComments)
	if err != nil {
		return fmt.Errorf("parse %s: %w", filePath, err)
	}
	return cg.ParseAST(f, cg.Fset, filePath, pkg)
}

// ParseAST processes an already-parsed AST, avoiding a second parse.
// fset must be the FileSet that was used to parse f, so position lookups are valid.
func (cg *CallGraph) ParseAST(f *ast.File, fset *token.FileSet, filePath, pkg string) error {
	ast.Inspect(f, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncDecl:
			cg.processFuncDecl(node, fset, filePath, pkg)
			return false // processFuncDecl walks fn.Body internally with the correct callerID
		case *ast.CallExpr:
			cg.processCallExpr(node, fset, filePath)
		}
		return true
	})
	return nil
}

func (cg *CallGraph) processFuncDecl(fn *ast.FuncDecl, fset *token.FileSet, filePath, pkg string) {
	pos := fset.Position(fn.Pos())
	recv := ""
	if fn.Recv != nil && len(fn.Recv.List) > 0 {
		recv = receiverType(fn.Recv.List[0].Type)
	}
	node := &Node{
		Pkg:  pkg,
		Name: fn.Name.Name,
		Recv: recv,
		File: filePath,
		Line: pos.Line,
	}
	cg.AddNode(node)
	callerID := node.ID()

	if fn.Body != nil {
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			calleeID := extractCallName(call)
			if calleeID != "" && calleeID != callerID {
				callPos := fset.Position(call.Pos())
				cg.AddEdge(callerID, calleeID, callPos.Line, filePath)
			}
			return false
		})
	}
}

func (cg *CallGraph) processCallExpr(call *ast.CallExpr, fset *token.FileSet, filePath string) {
	calleeID := extractCallName(call)
	if calleeID != "" {
		pos := fset.Position(call.Pos())
		cg.AddEdge(":<package-init>", calleeID, pos.Line, filePath)
	}
}

func (cg *CallGraph) TraverseBFS(start string, maxDepth int) []string {
	visited := make(map[string]bool)
	var result []string
	type item struct {
		id    string
		depth int
	}
	queue := []item{{start, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.id] {
			continue
		}
		visited[cur.id] = true
		result = append(result, cur.id)
		if maxDepth > 0 && cur.depth >= maxDepth {
			continue
		}
		for callee := range cg.OutEdges[cur.id] {
			if !visited[callee] {
				queue = append(queue, item{callee, cur.depth + 1})
			}
		}
	}
	return result
}

func (cg *CallGraph) Stats() map[string]int {
	return map[string]int{
		"nodes": len(cg.Nodes),
		"edges": len(cg.EdgeMeta),
	}
}

func (cg *CallGraph) SortedNodes() []*Node {
	result := make([]*Node, 0, len(cg.Nodes))
	for _, n := range cg.Nodes {
		result = append(result, n)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].ID() < result[j].ID()
	})
	return result
}

func storageNode(n *Node) storage.GraphNode {
	return storage.GraphNode{
		ID:   n.ID(),
		Pkg:  n.Pkg,
		Name: n.Name,
		Recv: n.Recv,
		File: n.File,
		Line: n.Line,
	}
}

func (cg *CallGraph) load() {
	if cg.store == nil {
		return
	}
	nodes, err := cg.store.LoadNodes()
	if err != nil {
		return
	}
	for _, gn := range nodes {
		n := &Node{Pkg: gn.Pkg, Name: gn.Name, Recv: gn.Recv, File: gn.File, Line: gn.Line}
		cg.Nodes[n.ID()] = n
	}
	edges, err := cg.store.LoadEdges()
	if err != nil {
		return
	}
	for _, e := range edges {
		if cg.OutEdges[e.Caller] == nil {
			cg.OutEdges[e.Caller] = make(map[string]bool)
		}
		if cg.InEdges[e.Callee] == nil {
			cg.InEdges[e.Callee] = make(map[string]bool)
		}
		cg.OutEdges[e.Caller][e.Callee] = true
		cg.InEdges[e.Callee][e.Caller] = true
		key := e.Caller + "\x00" + e.Callee
		cg.EdgeMeta[key] = &edgeMeta{Line: e.Line, File: e.File}
	}
}

// --- ImportGraph with adjacency list ---

type ImportGraph struct {
	OutEdges map[string]map[string]bool // pkg -> set of deps
	InEdges  map[string]map[string]bool // dep -> set of pkgs
	store    *storage.GraphStore
}

func NewImportGraph() *ImportGraph {
	return &ImportGraph{
		OutEdges: make(map[string]map[string]bool),
		InEdges:  make(map[string]map[string]bool),
	}
}

func NewPersistentImportGraph(gs *storage.GraphStore) *ImportGraph {
	ig := NewImportGraph()
	ig.store = gs
	if gs != nil {
		ig.load()
	}
	return ig
}

func (ig *ImportGraph) AddFile(pkg, path, src string) error {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, src, parser.ImportsOnly)
	if err != nil {
		return err
	}
	return ig.AddAST(pkg, f)
}

// AddAST processes imports from an already-parsed AST, avoiding re-parsing.
func (ig *ImportGraph) AddAST(pkg string, f *ast.File) error {
	if ig.OutEdges[pkg] == nil {
		ig.OutEdges[pkg] = make(map[string]bool)
	}
	for _, imp := range f.Imports {
		importPath := strings.Trim(imp.Path.Value, "\"`")
		if !strings.Contains(importPath, ".") {
			continue
		}
		if !ig.OutEdges[pkg][importPath] {
			ig.OutEdges[pkg][importPath] = true
			if ig.InEdges[importPath] == nil {
				ig.InEdges[importPath] = make(map[string]bool)
			}
			ig.InEdges[importPath][pkg] = true
			if ig.store != nil {
				ig.store.SaveImport(pkg, importPath)
			}
		}
	}
	return nil
}

func (ig *ImportGraph) Dependencies(pkg string) []string {
	var deps []string
	for dep := range ig.OutEdges[pkg] {
		deps = append(deps, dep)
	}
	sort.Strings(deps)
	return deps
}

func (ig *ImportGraph) Dependents(dep string) []string {
	var pkgs []string
	for pkg := range ig.InEdges[dep] {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}

func (ig *ImportGraph) load() {
	if ig.store == nil {
		return
	}
	imports, err := ig.store.LoadImports()
	if err != nil {
		return
	}
	for pkg, deps := range imports {
		if ig.OutEdges[pkg] == nil {
			ig.OutEdges[pkg] = make(map[string]bool)
		}
		for dep := range deps {
			ig.OutEdges[pkg][dep] = true
			if ig.InEdges[dep] == nil {
				ig.InEdges[dep] = make(map[string]bool)
			}
			ig.InEdges[dep][pkg] = true
		}
	}
}

// --- parser helpers ---

func receiverType(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + receiverType(t.X)
	case *ast.IndexExpr:
		return receiverType(t.X) + "[" + receiverType(t.Index) + "]"
	default:
		return fmt.Sprintf("%T", expr)
	}
}

func extractCallName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.Ident:
		return fun.Name
	case *ast.SelectorExpr:
		if id, ok := fun.X.(*ast.Ident); ok {
			return fmt.Sprintf("%s.%s", id.Name, fun.Sel.Name)
		}
		return fun.Sel.Name
	case *ast.IndexExpr:
		if id, ok := fun.X.(*ast.Ident); ok {
			return id.Name
		}
		return ""
	case *ast.FuncLit:
		return "<anonymous>"
	default:
		return ""
	}
}
