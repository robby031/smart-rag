package storage

import (
	"encoding/json"
	"fmt"
	"strings"
)

const (
	graphNodePrefix = "graph:node:"
	graphEdgePrefix = "graph:edge:"
	graphMetaKey    = "graph:meta"
)

type GraphMeta struct {
	NodeCount int `json:"node_count"`
	EdgeCount int `json:"edge_count"`
}

type GraphNode struct {
	ID   string `json:"id"`
	Pkg  string `json:"pkg"`
	Name string `json:"name"`
	Recv string `json:"recv,omitempty"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type GraphEdge struct {
	Caller string `json:"caller"`
	Callee string `json:"callee"`
	Line   int    `json:"line"`
	File   string `json:"file"`
}

type GraphStore struct {
	kv *Store
}

func NewGraphStore(kv *Store) *GraphStore {
	return &GraphStore{kv: kv}
}

func (gs *GraphStore) SaveNode(node GraphNode) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal node: %w", err)
	}
	return gs.kv.Put([]byte(graphNodePrefix+node.ID), data)
}

func (gs *GraphStore) DeleteNode(id string) error {
	return gs.kv.Delete([]byte(graphNodePrefix + id))
}

func (gs *GraphStore) LoadNodes() (map[string]GraphNode, error) {
	raw, err := gs.kv.GetWithPrefix([]byte(graphNodePrefix))
	if err != nil {
		return nil, err
	}
	nodes := make(map[string]GraphNode, len(raw))
	for key, data := range raw {
		var node GraphNode
		if err := json.Unmarshal(data, &node); err != nil {
			return nil, fmt.Errorf("unmarshal node %s: %w", key, err)
		}
		nodes[node.ID] = node
	}
	return nodes, nil
}

func (gs *GraphStore) SaveEdge(edge GraphEdge) error {
	data, err := json.Marshal(edge)
	if err != nil {
		return fmt.Errorf("marshal edge: %w", err)
	}
	key := fmt.Sprintf("%s%s\x00%s", graphEdgePrefix, edge.Caller, edge.Callee)
	return gs.kv.Put([]byte(key), data)
}

func (gs *GraphStore) DeleteEdge(caller, callee string) error {
	key := fmt.Sprintf("%s%s\x00%s", graphEdgePrefix, caller, callee)
	return gs.kv.Delete([]byte(key))
}

func (gs *GraphStore) LoadEdges() ([]GraphEdge, error) {
	raw, err := gs.kv.GetWithPrefix([]byte(graphEdgePrefix))
	if err != nil {
		return nil, err
	}
	edges := make([]GraphEdge, 0, len(raw))
	for _, data := range raw {
		var edge GraphEdge
		if err := json.Unmarshal(data, &edge); err != nil {
			return nil, fmt.Errorf("unmarshal edge: %w", err)
		}
		edges = append(edges, edge)
	}
	return edges, nil
}

func (gs *GraphStore) SaveMeta(meta GraphMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal meta: %w", err)
	}
	return gs.kv.Put([]byte(graphMetaKey), data)
}

func (gs *GraphStore) LoadMeta() (*GraphMeta, error) {
	data, err := gs.kv.Get([]byte(graphMetaKey))
	if err != nil {
		return nil, err
	}
	var meta GraphMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal meta: %w", err)
	}
	return &meta, nil
}

const importPrefix = "import:"

func (gs *GraphStore) SaveImport(pkg, dep string) error {
	key := importPrefix + pkg + "\x00" + dep
	return gs.kv.Put([]byte(key), []byte(dep))
}

func (gs *GraphStore) LoadImports() (map[string]map[string]bool, error) {
	raw, err := gs.kv.GetWithPrefix([]byte(importPrefix))
	if err != nil {
		return nil, err
	}
	result := make(map[string]map[string]bool)
	for key := range raw {
		rest := key[len(importPrefix):]
		before, after, ok := strings.Cut(rest, "\x00")
		if !ok {
			continue
		}
		pkg := before
		dep := after
		if result[pkg] == nil {
			result[pkg] = make(map[string]bool)
		}
		result[pkg][dep] = true
	}
	return result, nil
}

func (gs *GraphStore) SaveNodeBatch(nodes []GraphNode) error {
	pairs := make([]KVPair, 0, len(nodes))
	for _, node := range nodes {
		data, err := json.Marshal(node)
		if err != nil {
			return fmt.Errorf("marshal node: %w", err)
		}
		pairs = append(pairs, KVPair{Key: []byte(graphNodePrefix + node.ID), Value: data})
	}
	return gs.kv.BatchPut(pairs)
}

func (gs *GraphStore) SaveEdgeBatch(edges []GraphEdge) error {
	pairs := make([]KVPair, 0, len(edges))
	for _, edge := range edges {
		data, err := json.Marshal(edge)
		if err != nil {
			return fmt.Errorf("marshal edge: %w", err)
		}
		key := fmt.Sprintf("%s%s\x00%s", graphEdgePrefix, edge.Caller, edge.Callee)
		pairs = append(pairs, KVPair{Key: []byte(key), Value: data})
	}
	return gs.kv.BatchPut(pairs)
}

func (gs *GraphStore) SaveImportBatch(pairs [][2]string) error {
	kvPairs := make([]KVPair, 0, len(pairs))
	for _, p := range pairs {
		key := importPrefix + p[0] + "\x00" + p[1]
		kvPairs = append(kvPairs, KVPair{Key: []byte(key), Value: []byte(p[1])})
	}
	return gs.kv.BatchPut(kvPairs)
}

func (gs *GraphStore) DeleteNodesByIDs(ids []string) error {
	return gs.kv.BatchDelete(idsToKeys(ids, graphNodePrefix))
}

func (gs *GraphStore) DeleteEdgesByCaller(callerID string) error {
	prefix := graphEdgePrefix + callerID + "\x00"
	return gs.kv.DeleteWithPrefix([]byte(prefix))
}

func (gs *GraphStore) DeleteEdgesByCallers(callerIDs []string) error {
	prefixes := make([][]byte, len(callerIDs))
	for i, id := range callerIDs {
		prefixes[i] = []byte(graphEdgePrefix + id + "\x00")
	}
	return gs.kv.DeleteWithPrefixes(prefixes)
}

func MarshalNodeKV(node GraphNode) KVPair {
	data, _ := json.Marshal(node)
	return KVPair{Key: []byte(graphNodePrefix + node.ID), Value: data}
}

func MarshalEdgeKV(edge GraphEdge) KVPair {
	data, _ := json.Marshal(edge)
	key := fmt.Sprintf("%s%s\x00%s", graphEdgePrefix, edge.Caller, edge.Callee)
	return KVPair{Key: []byte(key), Value: data}
}

func (gs *GraphStore) FlushBatch(deleteKeys [][]byte, deletePrefixes [][]byte, puts []KVPair) error {
	return gs.kv.FlushBatch(deleteKeys, deletePrefixes, puts)
}

func idsToKeys(ids []string, prefix string) [][]byte {
	keys := make([][]byte, len(ids))
	for i, id := range ids {
		keys[i] = []byte(prefix + id)
	}
	return keys
}

func (gs *GraphStore) ClearGraph() error {
	prefixes := []string{graphNodePrefix, graphEdgePrefix, importPrefix}
	for _, prefix := range prefixes {
		raw, err := gs.kv.GetWithPrefix([]byte(prefix))
		if err != nil {
			return err
		}
		for key := range raw {
			if err := gs.kv.Delete([]byte(key)); err != nil {
				return err
			}
		}
	}
	return gs.kv.Delete([]byte(graphMetaKey))
}
