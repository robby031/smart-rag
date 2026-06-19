package graph

import (
	"fmt"
	"sort"
	"strings"
)

type Graph struct {
	callGraph   *CallGraph
	importGraph *ImportGraph
}

func NewGraph(cg *CallGraph, ig *ImportGraph) *Graph {
	return &Graph{callGraph: cg, importGraph: ig}
}

func (g *Graph) Callers(funcID string) []string {
	return g.callGraph.Callers(funcID)
}

func (g *Graph) Callees(funcID string) []string {
	return g.callGraph.Callees(funcID)
}

func (g *Graph) Importers(pkgPath string) []string {
	return g.importGraph.Dependents(pkgPath)
}

func (g *Graph) Dependencies(pkg string) []string {
	return g.importGraph.Dependencies(pkg)
}

type ImpactResult struct {
	ID    string `json:"id"`
	Depth int    `json:"depth"`
	Dir   string `json:"dir"`
	File  string `json:"file,omitempty"`
	Line  int    `json:"line,omitempty"`
}

func (g *Graph) ImpactForward(start string, maxDepth int) []ImpactResult {
	visited := make(map[string]bool)
	var results []ImpactResult
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
		if cur.depth > 0 {
			results = append(results, ImpactResult{ID: cur.id, Depth: cur.depth, Dir: "downstream"})
		}
		if maxDepth > 0 && cur.depth >= maxDepth {
			continue
		}
		for _, callee := range g.callGraph.Callees(cur.id) {
			if !visited[callee] {
				queue = append(queue, item{callee, cur.depth + 1})
			}
		}
	}
	return results
}

func (g *Graph) ImpactBackward(start string, maxDepth int) []ImpactResult {
	visited := make(map[string]bool)
	var results []ImpactResult
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
		if cur.depth > 0 {
			results = append(results, ImpactResult{ID: cur.id, Depth: cur.depth, Dir: "upstream"})
		}
		if maxDepth > 0 && cur.depth >= maxDepth {
			continue
		}
		for _, caller := range g.callGraph.Callers(cur.id) {
			if !visited[caller] {
				queue = append(queue, item{caller, cur.depth + 1})
			}
		}
	}
	return results
}

func (g *Graph) ImpactFull(start string, maxDepth int) []ImpactResult {
	var results []ImpactResult
	results = append(results, g.ImpactForward(start, maxDepth)...)
	results = append(results, g.ImpactBackward(start, maxDepth)...)
	return results
}

func (g *Graph) PackageImpact(pkg string, maxDepth int) []ImpactResult {
	visited := make(map[string]bool)
	var results []ImpactResult
	type item struct {
		pkg   string
		depth int
	}
	queue := []item{{pkg, 0}}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		if visited[cur.pkg] {
			continue
		}
		visited[cur.pkg] = true
		if cur.depth > 0 {
			results = append(results, ImpactResult{ID: cur.pkg, Depth: cur.depth, Dir: "downstream"})
		}
		if maxDepth > 0 && cur.depth >= maxDepth {
			continue
		}
		for _, importer := range g.importGraph.Dependents(cur.pkg) {
			if !visited[importer] {
				queue = append(queue, item{importer, cur.depth + 1})
			}
		}
	}
	return results
}

type XrefResult struct {
	Symbol      string   `json:"symbol"`
	Definitions []string `json:"definitions,omitempty"`
	References  []string `json:"references,omitempty"`
	Callers     []string `json:"callers,omitempty"`
	Callees     []string `json:"callees,omitempty"`
	ImportedBy  []string `json:"imported_by,omitempty"`
}

func (g *Graph) Xref(symbol string) *XrefResult {
	g.callGraph.mu.Lock()
	defer g.callGraph.mu.Unlock()

	r := &XrefResult{Symbol: symbol}

	if node, ok := g.callGraph.Nodes[symbol]; ok {
		r.Definitions = append(r.Definitions, fmt.Sprintf("%s (%s:%d)", node.ID(), node.File, node.Line))
	}

	if len(r.Definitions) == 0 {
		for _, n := range g.callGraph.SortedNodes() {
			if n.Name == symbol || strings.Contains(n.ID(), symbol) {
				r.Definitions = append(r.Definitions, fmt.Sprintf("%s (%s:%d)", n.ID(), n.File, n.Line))
			}
		}
	}

	r.Callers = g.callGraph.Callers(symbol)
	r.Callees = g.callGraph.Callees(symbol)
	r.ImportedBy = g.importGraph.Dependents(symbol)

	r.References = append(r.References, r.Callers...)
	r.References = append(r.References, r.ImportedBy...)

	return r
}

func (g *Graph) SearchSymbol(query string) []*Node {
	g.callGraph.mu.Lock()
	defer g.callGraph.mu.Unlock()

	var results []*Node
	for _, n := range g.callGraph.Nodes {
		if strings.Contains(strings.ToLower(n.Name), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(n.Pkg), strings.ToLower(query)) ||
			strings.Contains(strings.ToLower(n.ID()), strings.ToLower(query)) {
			results = append(results, n)
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].ID() < results[j].ID()
	})
	return results
}
