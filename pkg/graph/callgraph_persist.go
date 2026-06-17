package graph

import (
	"sort"

	"github.com/robby031/smart-rag/pkg/storage"
)

func (cg *CallGraph) Flush() error {
	if cg.store == nil {
		return nil
	}
	if len(cg.dirtyNodes) > 0 {
		nodes := make([]storage.GraphNode, 0, len(cg.dirtyNodes))
		for id := range cg.dirtyNodes {
			if n, ok := cg.Nodes[id]; ok {
				nodes = append(nodes, storageNode(n))
			}
		}
		if err := cg.store.SaveNodeBatch(nodes); err != nil {
			return err
		}
		cg.dirtyNodes = make(map[string]bool)
	}
	if len(cg.dirtyCallers) > 0 {
		var edgeCount int
		for caller := range cg.dirtyCallers {
			edgeCount += len(cg.OutEdges[caller])
		}
		edges := make([]storage.GraphEdge, 0, edgeCount)
		for caller := range cg.dirtyCallers {
			for callee := range cg.OutEdges[caller] {
				edges = append(edges, storage.GraphEdge{Caller: caller, Callee: callee})
			}
		}
		if err := cg.store.SaveEdgeBatch(edges); err != nil {
			return err
		}
		cg.dirtyCallers = make(map[string]bool)
	}
	return nil
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
	}
}
