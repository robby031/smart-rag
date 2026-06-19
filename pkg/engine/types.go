package engine

import (
	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/storage"
)

type QueryType int

const (
	QuerySearch QueryType = iota
	QueryDefinition
	QueryReferences
	QueryCallers
	QueryCallees
	QueryImpact
	QueryContextPack
	QueryReadSnippet
	QueryTraceVariable
	QueryFunctionFlow
	QueryTypeProvenance
	QueryVariableSearch
	QueryDynamicFlow
)

type Query struct {
	Type      QueryType
	Text      string
	File      string
	Language  string
	TopK      int
	MaxDepth  int
	MaxTokens int
}

type Result struct {
	Score   float64              `json:"score,omitempty"`
	Chunk   *storage.ChunkMeta   `json:"chunk,omitempty"`
	Node    *graph.Node          `json:"node,omitempty"`
	Impact  []graph.ImpactResult `json:"impact,omitempty"`
	Related []string             `json:"related,omitempty"`
	Content string               `json:"content,omitempty"`
}

type Response struct {
	Query   string   `json:"query"`
	Type    string   `json:"type"`
	Results []Result `json:"results"`
}
