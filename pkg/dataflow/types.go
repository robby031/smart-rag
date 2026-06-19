package dataflow

type ScopeType int

const (
	ScopeLocal ScopeType = iota
	ScopeParam
	ScopeGlobal
	ScopeField
)

type UseKind int

const (
	UseRead UseKind = iota
	UseWrite
	UseCallArg
	UseReturn
)

type Variable struct {
	Name     string    `json:"name"`
	Type     string    `json:"type"`
	Scope    ScopeType `json:"scope"`
	Pkg      string    `json:"pkg"`
	File     string    `json:"file"`
	DefLine  int       `json:"def_line"`
	DefChunk string    `json:"def_chunk"`
}

type VarDef struct {
	ID        string `json:"id"`
	Variable  string `json:"variable"`
	Pkg       string `json:"pkg"`
	File      string `json:"file"`
	StartLine int    `json:"start_line"`
	EndLine   int    `json:"end_line"`
	ChunkID   string `json:"chunk_id"`
}

type VarUse struct {
	ID       string  `json:"id"`
	Variable string  `json:"variable"`
	File     string  `json:"file"`
	Line     int     `json:"line"`
	Kind     UseKind `json:"kind"`
	FuncID   string  `json:"func_id"`
}

type DefUseChain struct {
	Def  VarDef   `json:"def"`
	Uses []VarUse `json:"uses"`
}

type TypeFlowNode struct {
	TypeName     string   `json:"type_name"`
	DefFile      string   `json:"def_file"`
	DefLine      int      `json:"def_line"`
	UsedAsParam  []string `json:"used_as_param"`
	UsedAsReturn []string `json:"used_as_return"`
	UsedAsField  []string `json:"used_as_field"`
}

type DataFlowEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"`
	File string `json:"file"`
	Line int    `json:"line"`
}

type FlowSummary struct {
	FuncID      string        `json:"func_id"`
	Inputs      []Variable    `json:"inputs"`
	Outputs     []Variable    `json:"outputs"`
	Internals   []DefUseChain `json:"internals"`
	SideEffects []string      `json:"side_effects"`
}

type FlowGraph struct {
	Variables map[string]*Variable     `json:"variables"`
	Defs      map[string]*VarDef       `json:"defs"`
	Uses      map[string]*VarUse       `json:"uses"`
	DefUseMap map[string]*DefUseChain  `json:"def_use_map"`
	TypeNodes map[string]*TypeFlowNode `json:"type_nodes"`
	Edges     []DataFlowEdge           `json:"edges"`
}

type FlowMeta struct {
	VariableCount int `json:"variable_count"`
	DefCount      int `json:"def_count"`
	UseCount      int `json:"use_count"`
	ChainCount    int `json:"chain_count"`
	TypeNodeCount int `json:"type_node_count"`
	EdgeCount     int `json:"edge_count"`
}
