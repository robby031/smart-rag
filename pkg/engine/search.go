package engine

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/robby031/smart-rag/pkg/storage"
)

func (e *Engine) search(_ context.Context, q Query, resp *Response) (*Response, error) {
	tokens := e.tokenizer.TokenizeQuery(q.Text)
	freq := make(map[string]int)
	for tok, count := range tokens {
		freq[tok] = count
	}

	topK := q.TopK
	if topK <= 0 {
		topK = 10
	}

	fetchK := topK
	if q.Language != "" || q.File != "" {
		fetchK = topK * 5
		if fetchK < 50 {
			fetchK = 50
		}
	}
	if fetchK < topK {
		fetchK = topK
	}

	queryReachable := e.queryReachableChunkSet(q.Text, tokens)
	var candidates []Result
	for _, sr := range e.bm25.Search(freq, fetchK) {
		chunk, err := e.chunkStore.Get(sr.ID)
		if err != nil || chunk == nil {
			continue
		}
		if q.Language != "" && !strings.HasSuffix(chunk.FilePath, "."+q.Language) {
			continue
		}
		if q.File != "" && !strings.Contains(chunk.FilePath, q.File) {
			continue
		}
		score, details := rankSearchResult(q, tokens, sr.Score, chunk, queryReachable)
		candidates = append(candidates, Result{
			Score:   score,
			Chunk:   chunk,
			Content: chunk.Content,
			Related: details,
		})
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].Score != candidates[j].Score {
			return candidates[i].Score > candidates[j].Score
		}
		return compareSearchTie(candidates[i], candidates[j])
	})
	if len(candidates) > topK {
		candidates = candidates[:topK]
	}
	resp.Results = append(resp.Results, candidates...)
	return resp, nil
}

func rankSearchResult(q Query, queryTokens map[string]int, bm25Score float64, chunk *storage.ChunkMeta, queryReachable map[string]bool) (float64, []string) {
	score := bm25Score
	details := []string{fmt.Sprintf("score bm25=%.4f", bm25Score)}

	symbol := strings.ToLower(chunk.SymbolName)
	if symbol != "" && normalizeSearchText(q.Text) == normalizeSearchText(chunk.SymbolName) {
		score += 2.0
		details = append(details, "boost exact_symbol=2.0000")
	}

	var symbolBoost float64
	for term := range queryTokens {
		if symbol != "" && strings.Contains(symbol, term) {
			symbolBoost += 0.05
		}
	}
	if symbolBoost > 0.15 {
		symbolBoost = 0.15
	}
	if symbolBoost > 0 {
		score += symbolBoost
		details = append(details, fmt.Sprintf("boost symbol_name=%.4f", symbolBoost))
	}

	filePath := strings.ToLower(chunk.FilePath)
	var pathBoost float64
	for term := range queryTokens {
		if strings.Contains(filePath, term) {
			pathBoost += 0.04
		}
	}
	if pathBoost > 0.12 {
		pathBoost = 0.12
	}
	if pathBoost > 0 {
		score += pathBoost
		details = append(details, fmt.Sprintf("boost file_path=%.4f", pathBoost))
	}

	if q.Language != "" && strings.HasSuffix(chunk.FilePath, "."+q.Language) {
		score += 0.03
		details = append(details, "boost language_filter=0.0300")
	}
	if q.File != "" && strings.Contains(chunk.FilePath, q.File) {
		score += 0.05
		details = append(details, "boost path_filter=0.0500")
	}

	if queryReachable[chunk.ID] && chunk.Reachability == ReachabilityUnreachable {
		details = append(details, "boost query_reachable_root=skip_unreachable_penalty")
	} else if weight := chunkContextWeight(chunk); weight < 1 {
		score *= weight
		if chunk.SemanticRole == SemanticRoleBoilerplate && chunk.FoldReason != "" {
			details = append(details, fmt.Sprintf("penalty semantic_role=%s fold_reason=%s weight=%.4f", chunk.SemanticRole, chunk.FoldReason, weight))
		} else {
			details = append(details, fmt.Sprintf("penalty reachability=%s weight=%.4f", valueOrUnknown(chunk.Reachability), weight))
		}
	}

	details[0] = fmt.Sprintf("%s final=%.4f", details[0], score)
	return score, details
}

func compareSearchTie(a, b Result) bool {
	if a.Chunk == nil || b.Chunk == nil {
		return a.Chunk != nil
	}
	if a.Chunk.FilePath != b.Chunk.FilePath {
		return a.Chunk.FilePath < b.Chunk.FilePath
	}
	if a.Chunk.SymbolName != b.Chunk.SymbolName {
		return a.Chunk.SymbolName < b.Chunk.SymbolName
	}
	return a.Chunk.ID < b.Chunk.ID
}

func normalizeSearchText(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func valueOrUnknown(s string) string {
	if s == "" {
		return ReachabilityUnknown
	}
	return s
}
