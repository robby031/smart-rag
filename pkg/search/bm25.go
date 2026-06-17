package search

import (
	"math"
	"sort"
)

type BM25 struct {
	docCount    int
	avgDocLen   float64
	docLen      []int
	termFreqs   []map[string]int
	idf         map[string]float64
	k1          float64
	b           float64
	DocIDs      []string
	lenNorm     []float64        // pre-computed: (1 - b + b * docLen / avgDocLen)
	termPosting map[string][]int // term -> list of doc indices containing it
}

func NewBM25() *BM25 {
	return &BM25{
		k1:          1.2,
		b:           0.75,
		termPosting: make(map[string][]int),
	}
}

func (b *BM25) AddDocument(tokens map[string]int, docID string) {
	idx := b.docCount
	b.termFreqs = append(b.termFreqs, tokens)
	b.DocIDs = append(b.DocIDs, docID)
	var total int
	for term := range tokens {
		total += tokens[term]
		b.termPosting[term] = append(b.termPosting[term], idx)
	}
	b.docLen = append(b.docLen, total)
	b.docCount++
}

func (b *BM25) Build() {
	if b.docCount == 0 {
		return
	}
	var totalLen int
	for _, l := range b.docLen {
		totalLen += l
	}
	b.avgDocLen = float64(totalLen) / float64(b.docCount)

	df := make(map[string]int, len(b.termPosting))
	for term, posting := range b.termPosting {
		df[term] = len(posting)
	}
	b.idf = make(map[string]float64, len(df))
	for term, count := range df {
		b.idf[term] = math.Log(1.0 + float64(b.docCount-count+1)/(float64(count)+0.5))
	}

	b.lenNorm = make([]float64, b.docCount)
	for i, l := range b.docLen {
		b.lenNorm[i] = 1 - b.b + b.b*float64(l)/b.avgDocLen
	}
}

type ScoredDoc struct {
	Index int
	ID    string
	Score float64
}

func (b *BM25) Score(query map[string]int) []ScoredDoc {
	if b.docCount == 0 {
		return nil
	}

	// Collect candidate docs that match at least one query term
	candidates := make(map[int]float64)
	for term := range query {
		for _, idx := range b.termPosting[term] {
			if _, seen := candidates[idx]; !seen {
				candidates[idx] = 0
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	for term := range query {
		idf := b.idf[term]
		if idf == 0 {
			continue
		}
		for idx := range candidates {
			tf := b.termFreqs[idx][term]
			if tf == 0 {
				continue
			}
			numer := float64(tf) * (b.k1 + 1)
			denom := float64(tf) + b.k1*b.lenNorm[idx]
			candidates[idx] += idf * numer / denom
		}
	}

	results := make([]ScoredDoc, 0, len(candidates))
	for idx, score := range candidates {
		results = append(results, ScoredDoc{Index: idx, ID: b.DocIDs[idx], Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func (b *BM25) scoreDoc(docIdx int, query map[string]int) float64 {
	var score float64
	for term := range query {
		tf := b.termFreqs[docIdx][term]
		if tf == 0 {
			continue
		}
		idf := b.idf[term]
		numer := float64(tf) * (b.k1 + 1)
		denom := float64(tf) + b.k1*b.lenNorm[docIdx]
		score += idf * numer / denom
	}
	return score
}

func (b *BM25) Search(query map[string]int, topK int) []ScoredDoc {
	scored := b.Score(query)
	if len(scored) <= topK {
		return scored
	}
	return scored[:topK]
}
