package search

import (
	"math"
	"sort"
	"sync"
)

type sparsePosting struct {
	docIdx int32
	weight float32
}

type SparseRetriever struct {
	mu          sync.Mutex
	docCount    int
	DocIDs      []string
	norms       []float64
	termPosting map[string][]sparsePosting
	idf         map[string]float64
}

func NewSparseRetriever() *SparseRetriever {
	return &SparseRetriever{
		idf:         make(map[string]float64),
		termPosting: make(map[string][]sparsePosting),
	}
}

func (s *SparseRetriever) AddDocument(vec map[string]float64, docID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := int32(s.docCount)
	s.DocIDs = append(s.DocIDs, docID)
	for term, val := range vec {
		s.termPosting[term] = append(s.termPosting[term], sparsePosting{docIdx: idx, weight: float32(val)})
	}
	s.docCount++
}

func (s *SparseRetriever) Build() {
	if s.docCount == 0 {
		return
	}
	for term, posts := range s.termPosting {
		df := len(posts)
		s.idf[term] = math.Log(1.0 + float64(s.docCount)/float64(df))
	}

	s.norms = make([]float64, s.docCount)
	for term, posts := range s.termPosting {
		idf := s.idf[term]
		for i, p := range posts {
			w := float64(p.weight) * idf
			s.termPosting[term][i].weight = float32(w)
			s.norms[p.docIdx] += w * w
		}
	}
	for i := range s.norms {
		s.norms[i] = math.Sqrt(s.norms[i])
	}
}

type ScoredResult struct {
	Index int
	ID    string
	Score float64
}

func (s *SparseRetriever) Search(query map[string]float64, topK int) []ScoredResult {
	if len(query) == 0 || s.docCount == 0 {
		return nil
	}

	var queryNorm float64
	for _, v := range query {
		queryNorm += v * v
	}
	queryNorm = math.Sqrt(queryNorm)
	if queryNorm == 0 {
		return nil
	}

	candidates := make(map[int32]float64)
	for term, qv := range query {
		for _, p := range s.termPosting[term] {
			candidates[p.docIdx] += qv * float64(p.weight)
		}
	}

	results := make([]ScoredResult, 0, len(candidates))
	for idx, dot := range candidates {
		if s.norms[idx] > 0 {
			score := dot / (queryNorm * s.norms[idx])
			if score > 0 {
				results = append(results, ScoredResult{Index: int(idx), ID: s.DocIDs[idx], Score: score})
			}
		}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	if topK <= 0 || topK > len(results) {
		topK = len(results)
	}
	return results[:topK]
}
