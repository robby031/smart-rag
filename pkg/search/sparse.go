package search

import (
	"math"
	"sort"
	"sync"
)

type SparseRetriever struct {
	mu          sync.Mutex
	vectors     []map[string]float64
	idf         map[string]float64
	docCount    int
	DocIDs      []string
	norms       []float64
	termPosting map[string][]int
}

func NewSparseRetriever() *SparseRetriever {
	return &SparseRetriever{
		idf:         make(map[string]float64),
		termPosting: make(map[string][]int),
	}
}

func (s *SparseRetriever) AddDocument(vec map[string]float64, docID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	idx := s.docCount
	s.vectors = append(s.vectors, vec)
	s.DocIDs = append(s.DocIDs, docID)
	for term := range vec {
		s.termPosting[term] = append(s.termPosting[term], idx)
	}
	s.docCount++
}

func (s *SparseRetriever) Build() {
	if s.docCount == 0 {
		return
	}
	df := make(map[string]int, len(s.termPosting))
	for term, posting := range s.termPosting {
		df[term] = len(posting)
	}
	for term, count := range df {
		s.idf[term] = math.Log(1.0 + float64(s.docCount)/float64(count))
	}

	s.norms = make([]float64, s.docCount)
	for i, vec := range s.vectors {
		weighted := make(map[string]float64, len(vec))
		var norm float64
		for term, val := range vec {
			w := val * s.idf[term]
			weighted[term] = w
			norm += w * w
		}
		s.vectors[i] = weighted
		s.norms[i] = math.Sqrt(norm)
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

	// Find candidate docs that share at least one query term
	candidates := make(map[int]float64)
	for term := range query {
		for _, idx := range s.termPosting[term] {
			if _, seen := candidates[idx]; !seen {
				candidates[idx] = 0
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	// Compute query norm
	var queryNorm float64
	for _, v := range query {
		queryNorm += v * v
	}
	queryNorm = math.Sqrt(queryNorm)
	if queryNorm == 0 {
		return nil
	}

	for idx := range candidates {
		var dot float64
		vec := s.vectors[idx]
		for term := range query {
			if v, ok := vec[term]; ok {
				dot += query[term] * v
			}
		}
		if s.norms[idx] > 0 {
			candidates[idx] = dot / (queryNorm * s.norms[idx])
		}
	}

	results := make([]ScoredResult, 0, len(candidates))
	for idx, score := range candidates {
		if score > 0 {
			results = append(results, ScoredResult{Index: idx, ID: s.DocIDs[idx], Score: score})
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
