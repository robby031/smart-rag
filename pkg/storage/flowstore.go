package storage

import (
	"encoding/json"
	"fmt"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

const (
	flowVarPrefix   = "flow:var:"
	flowDefPrefix   = "flow:def:"
	flowUsePrefix   = "flow:use:"
	flowChainPrefix = "flow:chain:"
	flowTypePrefix  = "flow:type:"
	flowEdgePrefix  = "flow:edge:"
	flowFuncPrefix  = "flow:func:"
	flowMetaKey     = "flow:meta"
)

type FlowStore struct {
	kv *Store
}

func NewFlowStore(kv *Store) *FlowStore {
	return &FlowStore{kv: kv}
}

func (fs *FlowStore) SaveVariable(v *dataflow.Variable) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("marshal variable: %w", err)
	}
	key := flowVarPrefix + v.Pkg + "." + v.Name
	return fs.kv.Put([]byte(key), data)
}

func (fs *FlowStore) LoadVariable(pkg, name string) (*dataflow.Variable, error) {
	data, err := fs.kv.Get([]byte(flowVarPrefix + pkg + "." + name))
	if err != nil {
		return nil, err
	}
	var v dataflow.Variable
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, fmt.Errorf("unmarshal variable: %w", err)
	}
	return &v, nil
}

func (fs *FlowStore) LoadAllVariables() (map[string]*dataflow.Variable, error) {
	raw, err := fs.kv.GetWithPrefix([]byte(flowVarPrefix))
	if err != nil {
		return nil, err
	}
	result := make(map[string]*dataflow.Variable, len(raw))
	for _, data := range raw {
		var v dataflow.Variable
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("unmarshal variable: %w", err)
		}
		result[v.Pkg+"."+v.Name] = &v
	}
	return result, nil
}

func (fs *FlowStore) SaveDef(def *dataflow.VarDef) error {
	data, err := json.Marshal(def)
	if err != nil {
		return fmt.Errorf("marshal def: %w", err)
	}
	return fs.kv.Put([]byte(flowDefPrefix+def.ID), data)
}

func (fs *FlowStore) SaveUse(use *dataflow.VarUse) error {
	data, err := json.Marshal(use)
	if err != nil {
		return fmt.Errorf("marshal use: %w", err)
	}
	return fs.kv.Put([]byte(flowUsePrefix+use.ID), data)
}

func (fs *FlowStore) SaveChain(chain *dataflow.DefUseChain) error {
	data, err := json.Marshal(chain)
	if err != nil {
		return fmt.Errorf("marshal chain: %w", err)
	}
	return fs.kv.Put([]byte(flowChainPrefix+chain.Def.ID), data)
}

func (fs *FlowStore) SaveTypeNode(node *dataflow.TypeFlowNode) error {
	data, err := json.Marshal(node)
	if err != nil {
		return fmt.Errorf("marshal type node: %w", err)
	}
	return fs.kv.Put([]byte(flowTypePrefix+node.TypeName), data)
}

func (fs *FlowStore) LoadTypeNode(typeName string) (*dataflow.TypeFlowNode, error) {
	data, err := fs.kv.Get([]byte(flowTypePrefix + typeName))
	if err != nil {
		return nil, err
	}
	var node dataflow.TypeFlowNode
	if err := json.Unmarshal(data, &node); err != nil {
		return nil, fmt.Errorf("unmarshal type node: %w", err)
	}
	return &node, nil
}

func (fs *FlowStore) SaveEdge(edge *dataflow.DataFlowEdge) error {
	data, err := json.Marshal(edge)
	if err != nil {
		return fmt.Errorf("marshal edge: %w", err)
	}
	key := fmt.Sprintf("%s%s\x00%s", flowEdgePrefix, edge.From, edge.To)
	return fs.kv.Put([]byte(key), data)
}

func (fs *FlowStore) SaveFuncVariables(funcID string, varNames []string) error {
	data, err := json.Marshal(varNames)
	if err != nil {
		return fmt.Errorf("marshal func variables: %w", err)
	}
	return fs.kv.Put([]byte(flowFuncPrefix+funcID), data)
}

func (fs *FlowStore) LoadFuncVariables(funcID string) ([]string, error) {
	data, err := fs.kv.Get([]byte(flowFuncPrefix + funcID))
	if err != nil {
		return nil, err
	}
	var names []string
	if err := json.Unmarshal(data, &names); err != nil {
		return nil, fmt.Errorf("unmarshal func variables: %w", err)
	}
	return names, nil
}

func (fs *FlowStore) DeleteByFile(filePath string) error {
	var deleteKeys [][]byte

	raw, err := fs.kv.GetWithPrefix([]byte(flowVarPrefix))
	if err != nil {
		return err
	}
	for key, data := range raw {
		var v dataflow.Variable
		if err := json.Unmarshal(data, &v); err != nil {
			continue
		}
		if v.File == filePath {
			deleteKeys = append(deleteKeys, []byte(key))
		}
	}

	raw, err = fs.kv.GetWithPrefix([]byte(flowDefPrefix))
	if err != nil {
		return err
	}
	var defIDs []string
	for key, data := range raw {
		var def dataflow.VarDef
		if err := json.Unmarshal(data, &def); err != nil {
			continue
		}
		if def.File == filePath {
			deleteKeys = append(deleteKeys, []byte(key))
			defIDs = append(defIDs, def.ID)
		}
	}

	for _, defID := range defIDs {
		deleteKeys = append(deleteKeys, []byte(flowChainPrefix+defID))
	}

	raw, err = fs.kv.GetWithPrefix([]byte(flowUsePrefix))
	if err != nil {
		return err
	}
	for key, data := range raw {
		var use dataflow.VarUse
		if err := json.Unmarshal(data, &use); err != nil {
			continue
		}
		if use.File == filePath {
			deleteKeys = append(deleteKeys, []byte(key))
		}
	}

	raw, err = fs.kv.GetWithPrefix([]byte(flowEdgePrefix))
	if err != nil {
		return err
	}
	for key, data := range raw {
		var edge dataflow.DataFlowEdge
		if err := json.Unmarshal(data, &edge); err != nil {
			continue
		}
		if edge.File == filePath {
			deleteKeys = append(deleteKeys, []byte(key))
		}
	}

	raw, err = fs.kv.GetWithPrefix([]byte(flowTypePrefix))
	if err != nil {
		return err
	}
	for key, data := range raw {
		var node dataflow.TypeFlowNode
		if err := json.Unmarshal(data, &node); err != nil {
			continue
		}
		if node.DefFile == filePath {
			deleteKeys = append(deleteKeys, []byte(key))
		}
	}

	if len(deleteKeys) == 0 {
		return nil
	}
	return fs.kv.BatchDelete(deleteKeys)
}

func (fs *FlowStore) DeleteByFunc(funcID string) error {
	if err := fs.kv.Delete([]byte(flowFuncPrefix + funcID)); err != nil {
		return err
	}

	raw, err := fs.kv.GetWithPrefix([]byte(flowUsePrefix))
	if err != nil {
		return err
	}
	var useKeys [][]byte
	for key, data := range raw {
		var use dataflow.VarUse
		if err := json.Unmarshal(data, &use); err != nil {
			continue
		}
		if use.FuncID == funcID {
			useKeys = append(useKeys, []byte(key))
		}
	}
	if len(useKeys) > 0 {
		if err := fs.kv.BatchDelete(useKeys); err != nil {
			return err
		}
	}

	return nil
}

func (fs *FlowStore) SaveMeta(meta *dataflow.FlowMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal flow meta: %w", err)
	}
	return fs.kv.Put([]byte(flowMetaKey), data)
}

func (fs *FlowStore) LoadMeta() (*dataflow.FlowMeta, error) {
	data, err := fs.kv.Get([]byte(flowMetaKey))
	if err != nil {
		return nil, err
	}
	var meta dataflow.FlowMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal flow meta: %w", err)
	}
	return &meta, nil
}

func (fs *FlowStore) LoadAllDefs() (map[string]*dataflow.VarDef, error) {
	raw, err := fs.kv.GetWithPrefix([]byte(flowDefPrefix))
	if err != nil {
		return nil, err
	}
	result := make(map[string]*dataflow.VarDef, len(raw))
	for _, data := range raw {
		var def dataflow.VarDef
		if err := json.Unmarshal(data, &def); err != nil {
			return nil, fmt.Errorf("unmarshal def: %w", err)
		}
		result[def.ID] = &def
	}
	return result, nil
}

func (fs *FlowStore) LoadAllUses() (map[string]*dataflow.VarUse, error) {
	raw, err := fs.kv.GetWithPrefix([]byte(flowUsePrefix))
	if err != nil {
		return nil, err
	}
	result := make(map[string]*dataflow.VarUse, len(raw))
	for _, data := range raw {
		var use dataflow.VarUse
		if err := json.Unmarshal(data, &use); err != nil {
			return nil, fmt.Errorf("unmarshal use: %w", err)
		}
		result[use.ID] = &use
	}
	return result, nil
}

func (fs *FlowStore) LoadAllEdges() ([]dataflow.DataFlowEdge, error) {
	raw, err := fs.kv.GetWithPrefix([]byte(flowEdgePrefix))
	if err != nil {
		return nil, err
	}
	edges := make([]dataflow.DataFlowEdge, 0, len(raw))
	for _, data := range raw {
		var edge dataflow.DataFlowEdge
		if err := json.Unmarshal(data, &edge); err != nil {
			return nil, fmt.Errorf("unmarshal edge: %w", err)
		}
		edges = append(edges, edge)
	}
	return edges, nil
}
