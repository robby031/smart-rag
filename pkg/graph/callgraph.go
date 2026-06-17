package graph

import (
	"go/token"
	"sort"
	"sync"

	"github.com/robby031/smart-rag/pkg/storage"
)

type CallGraph struct {
	mu               sync.Mutex
	Nodes            map[string]*Node
	OutEdges         map[string]map[string]bool
	InEdges          map[string]map[string]bool
	Fset             *token.FileSet
	store            *storage.GraphStore
	dirtyNodes       map[string]bool
	dirtyCallers     map[string]bool
	deletedNodeIDs   []string
	deletedCallerIDs []string
}

func NewCallGraph() *CallGraph {
	return &CallGraph{
		Nodes:        make(map[string]*Node),
		OutEdges:     make(map[string]map[string]bool),
		InEdges:      make(map[string]map[string]bool),
		Fset:         token.NewFileSet(),
		dirtyNodes:   make(map[string]bool),
		dirtyCallers: make(map[string]bool),
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
	if existing, ok := cg.Nodes[id]; !ok || *existing != *n {
		cg.Nodes[id] = n
		cg.dirtyNodes[id] = true
	}
}

func (cg *CallGraph) AddEdge(caller, callee string, _ int, _ string) {
	if cg.OutEdges[caller] == nil {
		cg.OutEdges[caller] = make(map[string]bool)
	}
	if !cg.OutEdges[caller][callee] {
		cg.OutEdges[caller][callee] = true
		cg.dirtyCallers[caller] = true
	}
}

func (cg *CallGraph) BuildInEdges() {
	cg.InEdges = make(map[string]map[string]bool, len(cg.Nodes))
	for caller, callees := range cg.OutEdges {
		for callee := range callees {
			if cg.InEdges[callee] == nil {
				cg.InEdges[callee] = make(map[string]bool)
			}
			cg.InEdges[callee][caller] = true
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
	count := 0
	for _, callees := range cg.OutEdges {
		count += len(callees)
	}
	return count
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

func (cg *CallGraph) DeleteByFile(filePath string) {
	cg.mu.Lock()
	defer cg.mu.Unlock()

	var deleted []string
	for id, node := range cg.Nodes {
		if node.File == filePath {
			deleted = append(deleted, id)
		}
	}
	for _, id := range deleted {
		delete(cg.Nodes, id)
		delete(cg.OutEdges, id)
		delete(cg.dirtyNodes, id)
		delete(cg.dirtyCallers, id)
	}
	// Remove stale in-edge entries that pointed to the deleted callers.
	for callee := range cg.InEdges {
		for _, id := range deleted {
			delete(cg.InEdges[callee], id)
		}
	}
	if cg.store != nil && len(deleted) > 0 {
		cg.deletedNodeIDs = append(cg.deletedNodeIDs, deleted...)
		cg.deletedCallerIDs = append(cg.deletedCallerIDs, deleted...)
	}
}

func (cg *CallGraph) Stats() map[string]int {
	return map[string]int{
		"nodes": len(cg.Nodes),
		"edges": cg.EdgeCount(),
	}
}
