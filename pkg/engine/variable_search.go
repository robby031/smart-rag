package engine

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/robby031/smart-rag/pkg/dataflow"
)

type VariableSearchResult struct {
	Variable   string  `json:"variable"`
	Type       string  `json:"type,omitempty"`
	File       string  `json:"file"`
	Line       int     `json:"line"`
	FuncID     string  `json:"func_id,omitempty"`
	Score      float64 `json:"score"`
	Context    string  `json:"context,omitempty"`
	IsExported bool    `json:"is_exported"`
}

func (e *Engine) handleVariableSearch(q Query) (*Response, error) {
	query := strings.TrimSpace(q.Text)
	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	queryTokens := make(map[string]int)
	if query != "" {
		tokens := e.tokenizer.Tokenize(query)
		for tok, count := range tokens {
			queryTokens[tok] = count
		}
	}

	scored := e.searchScoredVariables(query, queryTokens)

	if q.File != "" {
		var filtered []VariableSearchResult
		for _, r := range scored {
			if strings.Contains(r.File, q.File) {
				filtered = append(filtered, r)
			}
		}
		scored = filtered
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].Score > scored[j].Score
	})

	if len(scored) > topK {
		scored = scored[:topK]
	}

	var results []Result
	for _, r := range scored {
		results = append(results, Result{
			Content: fmt.Sprintf("%+v", r),
		})
	}

	return &Response{
		Query:   q.Text,
		Type:    "variable_search",
		Results: results,
	}, nil
}

func (e *Engine) allIndexedVariables() []*dataflow.Variable {
	return e.flowIndex.SearchVariable("")
}

func (e *Engine) searchScoredVariables(query string, queryTokens map[string]int) []VariableSearchResult {
	allVars := e.allIndexedVariables()
	if allVars == nil {
		return nil
	}

	var scored []VariableSearchResult
	seen := make(map[string]bool)

	for _, v := range allVars {
		score := e.scoreVariable(v, query, queryTokens)
		if score <= 0 {
			continue
		}

		key := v.Name + ":" + v.File
		if seen[key] {
			continue
		}
		seen[key] = true

		scored = append(scored, VariableSearchResult{
			Variable:   v.Name,
			Type:       v.Type,
			File:       v.File,
			Line:       v.DefLine,
			Score:      score,
			IsExported: v.Name != "" && unicode.IsUpper(rune(v.Name[0])),
		})
	}
	return scored
}

func (e *Engine) scoreVariable(v *dataflow.Variable, query string, queryTokens map[string]int) float64 {
	score := 0.0
	queryLower := strings.ToLower(query)
	nameLower := strings.ToLower(v.Name)

	if nameLower == queryLower {
		score += 10.0
	} else if strings.Contains(nameLower, queryLower) {
		score += 5.0
	} else {
		nameTokens := e.tokenizer.Tokenize(v.Name)
		overlap := 0
		for qt := range queryTokens {
			for nt := range nameTokens {
				if qt == nt {
					overlap++
				}
			}
		}
		if overlap > 0 && len(queryTokens) > 0 {
			score += 2.0 * float64(overlap) / float64(len(queryTokens))
		}
	}

	if strings.Contains(strings.ToLower(v.Type), queryLower) {
		score += 3.0
	}

	if v.Name != "" && unicode.IsUpper(rune(v.Name[0])) {
		score += 1.5
	}

	return score
}
