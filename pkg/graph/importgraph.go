package graph

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"sync"

	"github.com/robby031/smart-rag/pkg/storage"
)

type ImportGraph struct {
	mu        sync.Mutex
	OutEdges  map[string]map[string]bool // pkg -> set of deps
	InEdges   map[string]map[string]bool // dep -> set of pkgs
	store     *storage.GraphStore
	dirtyPkgs map[string]bool // pkgs with new import edges since last Flush
}

func NewImportGraph() *ImportGraph {
	return &ImportGraph{
		OutEdges:  make(map[string]map[string]bool),
		InEdges:   make(map[string]map[string]bool),
		dirtyPkgs: make(map[string]bool),
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

func (ig *ImportGraph) AddAST(pkg string, f *ast.File) error {
	ig.mu.Lock()
	defer ig.mu.Unlock()
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
			ig.dirtyPkgs[pkg] = true
		}
	}
	return nil
}

func (ig *ImportGraph) Flush() error {
	if ig.store == nil || len(ig.dirtyPkgs) == 0 {
		return nil
	}
	var pairs [][2]string
	for pkg := range ig.dirtyPkgs {
		for dep := range ig.OutEdges[pkg] {
			pairs = append(pairs, [2]string{pkg, dep})
		}
	}
	if err := ig.store.SaveImportBatch(pairs); err != nil {
		return err
	}
	ig.dirtyPkgs = make(map[string]bool)
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
