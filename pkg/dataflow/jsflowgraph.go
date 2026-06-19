package dataflow

import "fmt"

type JSFlowGraphBuilder struct {
	extractor   *JSDefUseExtractor
	typeTracker *JSTypeFlowTracker
}

func NewJSFlowGraphBuilder() *JSFlowGraphBuilder {
	return &JSFlowGraphBuilder{
		extractor:   NewJSDefUseExtractor(),
		typeTracker: NewJSTypeFlowTracker(),
	}
}

func (b *JSFlowGraphBuilder) BuildFromSource(src, filePath, pkg string) (*FlowGraph, error) {
	chains, err := b.extractor.ExtractDefUse(src, filePath, pkg)
	if err != nil {
		return nil, fmt.Errorf("extract defuse: %w", err)
	}

	if err := b.typeTracker.ExtractTypes([]byte(src), filePath, pkg); err != nil {
		return nil, fmt.Errorf("extract types: %w", err)
	}

	fg := &FlowGraph{
		Variables: make(map[string]*Variable),
		Defs:      make(map[string]*VarDef),
		Uses:      make(map[string]*VarUse),
		DefUseMap: make(map[string]*DefUseChain),
		TypeNodes: b.typeTracker.GetAllNodes(),
		Edges:     nil,
	}

	for _, chain := range chains {
		def := chain.Def
		fg.Defs[def.ID] = &def
		fg.DefUseMap[def.ID] = chain

		if _, exists := fg.Variables[def.Variable]; !exists {
			fg.Variables[def.Variable] = &Variable{
				Name:    def.Variable,
				Pkg:     def.Pkg,
				File:    def.File,
				DefLine: def.StartLine,
			}
		}

		for _, use := range chain.Uses {
			u := use
			fg.Uses[u.ID] = &u
		}
	}

	var edges []DataFlowEdge
	for _, chain := range chains {
		for _, use := range chain.Uses {
			kind := "def_use"
			switch use.Kind {
			case UseCallArg:
				kind = "call_arg"
			case UseReturn:
				kind = "return_assign"
			}
			edges = append(edges, DataFlowEdge{
				From: chain.Def.ID,
				To:   use.ID,
				Kind: kind,
				File: use.File,
				Line: use.Line,
			})
		}
	}
	fg.Edges = edges

	return fg, nil
}
