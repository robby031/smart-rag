package dataflow

import "strings"

type FlowIndex struct {
	byName    map[string][]*Variable
	byType    map[string][]*Variable
	byFunc    map[string][]string
	byFile    map[string][]string
	chains    map[string]*DefUseChain
	typeNodes map[string]*TypeFlowNode
}

func NewFlowIndex() *FlowIndex {
	return &FlowIndex{
		byName:    make(map[string][]*Variable),
		byType:    make(map[string][]*Variable),
		byFunc:    make(map[string][]string),
		byFile:    make(map[string][]string),
		chains:    make(map[string]*DefUseChain),
		typeNodes: make(map[string]*TypeFlowNode),
	}
}

func (idx *FlowIndex) BuildFromFlowGraph(fg *FlowGraph) {
	idx.byName = make(map[string][]*Variable)
	idx.byType = make(map[string][]*Variable)
	idx.byFunc = make(map[string][]string)
	idx.byFile = make(map[string][]string)
	idx.chains = make(map[string]*DefUseChain)
	idx.typeNodes = make(map[string]*TypeFlowNode)

	for _, v := range fg.Variables {
		idx.byName[v.Name] = append(idx.byName[v.Name], v)
		if v.Type != "" {
			idx.byType[v.Type] = append(idx.byType[v.Type], v)
		}
		idx.byFile[v.File] = append(idx.byFile[v.File], v.Name)
	}

	for defID, chain := range fg.DefUseMap {
		idx.chains[defID] = chain
		for _, use := range chain.Uses {
			if use.FuncID != "" {
				idx.byFunc[use.FuncID] = append(idx.byFunc[use.FuncID], chain.Def.Variable)
			}
		}
	}

	for typeName, node := range fg.TypeNodes {
		idx.typeNodes[typeName] = node
	}
}

func (idx *FlowIndex) ByVariableName(name string) []*Variable {
	return idx.byName[name]
}

func (idx *FlowIndex) ByType(typeName string) []*Variable {
	return idx.byType[typeName]
}

func (idx *FlowIndex) ByFunction(funcID string) []*Variable {
	names := idx.byFunc[funcID]
	vars := make([]*Variable, 0, len(names))
	for _, name := range names {
		if v, ok := idx.byName[name]; ok {
			vars = append(vars, v...)
		}
	}
	return vars
}

func (idx *FlowIndex) ByFile(filePath string) []*Variable {
	names := idx.byFile[filePath]
	vars := make([]*Variable, 0, len(names))
	for _, name := range names {
		if v, ok := idx.byName[name]; ok {
			vars = append(vars, v...)
		}
	}
	return vars
}

func (idx *FlowIndex) GetChain(defID string) *DefUseChain {
	return idx.chains[defID]
}

func (idx *FlowIndex) GetAllChains() []*DefUseChain {
	chains := make([]*DefUseChain, 0, len(idx.chains))
	for _, c := range idx.chains {
		chains = append(chains, c)
	}
	return chains
}

func (idx *FlowIndex) GetTypeNode(typeName string) *TypeFlowNode {
	return idx.typeNodes[typeName]
}

func (idx *FlowIndex) SearchVariable(query string) []*Variable {
	lower := strings.ToLower(query)
	var results []*Variable
	seen := make(map[string]bool)
	for name, vars := range idx.byName {
		if !strings.Contains(strings.ToLower(name), lower) {
			continue
		}
		for _, v := range vars {
			key := v.Pkg + "." + v.Name + v.File
			if !seen[key] {
				results = append(results, v)
				seen[key] = true
			}
		}
	}
	return results
}
