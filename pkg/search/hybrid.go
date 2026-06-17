package search

import (
	"sort"
)

type HybridSearch struct {
	bm25   *BM25
	sparse *SparseRetriever
	alpha  float64
}

func NewHybridSearch(bm25 *BM25, sparse *SparseRetriever, alpha float64) *HybridSearch {
	if alpha < 0 {
		alpha = 0
	}
	if alpha > 1 {
		alpha = 1
	}
	return &HybridSearch{
		bm25:   bm25,
		sparse: sparse,
		alpha:  alpha,
	}
}

type HybridResult struct {
	Index       int
	ChunkID     string
	BM25Score   float64
	SparseScore float64
	FinalScore  float64
}

func (h *HybridSearch) Search(bm25Query map[string]int, sparseQuery map[string]float64, topK int) []HybridResult {
	bm25Results := h.bm25.Score(bm25Query)
	sparseResults := h.sparse.Search(sparseQuery, len(sparseQuery))

	bm25Score := make(map[string]float64)
	sparseScore := make(map[string]float64)
	bm25Raw := make(map[string]float64)
	sparseRaw := make(map[string]float64)

	const k = 60
	for i, r := range bm25Results {
		bm25Score[r.ID] = float64(1) / float64(k+i+1)
		bm25Raw[r.ID] = r.Score
	}
	for i, r := range sparseResults {
		sparseScore[r.ID] = float64(1) / float64(k+i+1)
		sparseRaw[r.ID] = r.Score
	}

	allIDs := make(map[string]bool)
	for _, r := range bm25Results {
		allIDs[r.ID] = true
	}
	for _, r := range sparseResults {
		allIDs[r.ID] = true
	}

	scoreMap := make(map[string]float64)
	for id := range allIDs {
		bs := bm25Score[id] * h.alpha
		ss := sparseScore[id] * (1 - h.alpha)

		if raw, ok := bm25Raw[id]; ok {
			bs += raw * h.alpha * 0.01
		}
		if raw, ok := sparseRaw[id]; ok {
			ss += raw * (1 - h.alpha) * 0.1
		}

		scoreMap[id] = bs + ss
	}

	results := make([]HybridResult, 0, len(scoreMap))
	for id, final := range scoreMap {
		results = append(results, HybridResult{
			ChunkID:     id,
			BM25Score:   bm25Raw[id],
			SparseScore: sparseRaw[id],
			FinalScore:  final,
		})
	}

	sort.Slice(results, func(i, j int) bool {
		return results[i].FinalScore > results[j].FinalScore
	})

	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	return results[:topK]
}
