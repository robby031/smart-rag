package search

import (
	"math"
	"sort"
)

type SparseRetriever struct {
	vectors  []map[string]float64
	idf      map[string]float64
	docCount int
	DocIDs   []string
}

func NewSparseRetriever() *SparseRetriever {
	return &SparseRetriever{
		idf: make(map[string]float64),
	}
}

func (s *SparseRetriever) AddDocument(vec map[string]float64, docID string) {
	s.vectors = append(s.vectors, vec)
	s.DocIDs = append(s.DocIDs, docID)
	s.docCount++
}

func (s *SparseRetriever) Build() {
	if s.docCount == 0 {
		return
	}
	df := make(map[string]int)
	for _, vec := range s.vectors {
		for term := range vec {
			df[term]++
		}
	}
	for term, count := range df {
		s.idf[term] = math.Log(1.0 + float64(s.docCount)/float64(count))
	}

	for i, vec := range s.vectors {
		weighted := make(map[string]float64, len(vec))
		for term, val := range vec {
			weighted[term] = val * s.idf[term]
		}
		s.vectors[i] = weighted
	}
}

type ScoredResult struct {
	Index int
	ID    string
	Score float64
}

func (s *SparseRetriever) Search(query map[string]float64, topK int) []ScoredResult {
	results := make([]ScoredResult, 0, s.docCount)

	for i, vec := range s.vectors {
		score := cosineSimilarity(query, vec)
		if score > 0 {
			results = append(results, ScoredResult{Index: i, ID: s.DocIDs[i], Score: score})
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

func cosineSimilarity(a, b map[string]float64) float64 {
	var dot, normA, normB float64

	for k, v := range a {
		normA += v * v
		if bv, ok := b[k]; ok {
			dot += v * bv
		}
	}
	for _, v := range b {
		normB += v * v
	}

	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
