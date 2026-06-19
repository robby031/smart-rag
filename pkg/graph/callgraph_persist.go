package graph

import (
	"log"
	"sort"

	"github.com/robby031/smart-rag/pkg/storage"
)

func (cg *CallGraph) Flush() error {
	if cg.store == nil {
		return nil
	}
	hasDirty := len(cg.deletedNodeIDs) > 0 || len(cg.deletedCallerIDs) > 0 ||
		len(cg.dirtyNodes) > 0 || len(cg.dirtyCallers) > 0
	if !hasDirty {
		return nil
	}

	var deleteNodeKeys [][]byte
	for _, id := range cg.deletedNodeIDs {
		deleteNodeKeys = append(deleteNodeKeys, []byte("graph:node:"+id))
	}

	var deleteEdgePrefixes [][]byte
	for _, callerID := range cg.deletedCallerIDs {
		deleteEdgePrefixes = append(deleteEdgePrefixes, []byte("graph:edge:"+callerID+"\x00"))
	}

	var savePairs []storage.KVPair
	for id := range cg.dirtyNodes {
		if n, ok := cg.Nodes[id]; ok {
			savePairs = append(savePairs, storage.MarshalNodeKV(storageNode(n)))
		}
	}
	for caller := range cg.dirtyCallers {
		for callee := range cg.OutEdges[caller] {
			savePairs = append(savePairs, storage.MarshalEdgeKV(storage.GraphEdge{Caller: caller, Callee: callee}))
		}
	}

	if err := cg.store.FlushBatch(deleteNodeKeys, deleteEdgePrefixes, savePairs); err != nil {
		return err
	}

	cg.deletedNodeIDs = nil
	cg.deletedCallerIDs = nil
	cg.dirtyNodes = make(map[string]bool)
	cg.dirtyCallers = make(map[string]bool)
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
		log.Printf("callgraph: failed to load nodes: %v", err)
		return
	}
	for _, gn := range nodes {
		n := &Node{Pkg: gn.Pkg, Name: gn.Name, Recv: gn.Recv, File: gn.File, Line: gn.Line}
		cg.Nodes[n.ID()] = n
	}
	edges, err := cg.store.LoadEdges()
	if err != nil {
		log.Printf("callgraph: failed to load edges (nodes already loaded, graph may be incomplete): %v", err)
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
