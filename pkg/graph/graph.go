package graph

import (
	"fmt"
	"sort"
	"strings"
)

// Graph unifies call graph and dependency graph into a single query interface.
type Graph struct {
	callGraph   *CallGraph
	importGraph *ImportGraph
}

// NewGraph creates a unified graph wrapping both call and import graphs.
func NewGraph(cg *CallGraph, ig *ImportGraph) *Graph {
	return &Graph{callGraph: cg, importGraph: ig}
}

// --- Call graph queries ---

// Callers returns all functions that call the given function ID.
func (g *Graph) Callers(funcID string) []string {
	return g.callGraph.Callers(funcID)
}

// Callees returns all functions called by the given function ID.
func (g *Graph) Callees(funcID string) []string {
	return g.callGraph.Callees(funcID)
}

// EdgeDetail returns file and line info for a caller->callee edge.
func (g *Graph) EdgeDetail(caller, callee string) (file string, line int, ok bool) {
	key := caller + "\x00" + callee
	em, found := g.callGraph.EdgeMeta[key]
	if found {
		return em.File, em.Line, true
	}
	return "", 0, false
}

// --- Import graph queries ---

// Importers returns packages that import the given package path.
func (g *Graph) Importers(pkgPath string) []string {
	return g.importGraph.Dependents(pkgPath)
}

// Dependencies returns packages imported by the given package.
func (g *Graph) Dependencies(pkg string) []string {
	return g.importGraph.Dependencies(pkg)
}

// --- Impact analysis (blast radius) ---

// ImpactResult describes one node in the impact chain.
type ImpactResult struct {
	ID    string `json:"id"`
	Depth int    `json:"depth"`
	Dir   string `json:"dir"` // "upstream" (callers) or "downstream" (callees)
	File  string `json:"file,omitempty"`
	Line  int    `json:"line,omitempty"`
}

// ImpactForward traces all callees (downstream) up to maxDepth.
// blast radius: "if I change this function, what else breaks?"
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

// ImpactBackward traces all callers (upstream) up to maxDepth.
// "what code depends on this function?"
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

// ImpactFull returns both upstream and downstream impact in one call.
func (g *Graph) ImpactFull(start string, maxDepth int) []ImpactResult {
	var results []ImpactResult
	results = append(results, g.ImpactForward(start, maxDepth)...)
	results = append(results, g.ImpactBackward(start, maxDepth)...)
	return results
}

// PackageImpact finds all packages affected by a change to the given package.
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

// --- Xref (cross-references) ---

// XrefResult holds all cross-references for a symbol.
type XrefResult struct {
	Symbol      string   `json:"symbol"`
	Definitions []string `json:"definitions,omitempty"`
	References  []string `json:"references,omitempty"`
	Callers     []string `json:"callers,omitempty"`
	Callees     []string `json:"callees,omitempty"`
	ImportedBy  []string `json:"imported_by,omitempty"`
}

// Xref resolves all cross-references for a given symbol or package.
// It searches across call graph and import graph.
func (g *Graph) Xref(symbol string) *XrefResult {
	r := &XrefResult{Symbol: symbol}

	// Check if it's a call graph node
	if node, ok := g.callGraph.Nodes[symbol]; ok {
		r.Definitions = append(r.Definitions, fmt.Sprintf("%s (%s:%d)", node.ID(), node.File, node.Line))
	}
	// If symbol doesn't match a full ID, try matching by name across all nodes
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

	// Collect all references: callers + importers
	r.References = append(r.References, r.Callers...)
	r.References = append(r.References, r.ImportedBy...)

	return r
}

// SearchSymbol finds call graph nodes matching the query by name/package.
func (g *Graph) SearchSymbol(query string) []*Node {
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

// --- Graph summary ---

// Summary returns a human-readable overview of the graph.
func (g *Graph) Summary() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Call graph: %d nodes, %d edges\n", len(g.callGraph.Nodes), len(g.callGraph.EdgeMeta)))
	b.WriteString(fmt.Sprintf("Import graph: %d packages\n", len(g.importGraph.OutEdges)))
	b.WriteString(fmt.Sprintf("Importers (reverse deps): %d packages\n", len(g.importGraph.InEdges)))
	return b.String()
}
