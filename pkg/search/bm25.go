package search

import (
	"math"
	"sort"
)

type BM25 struct {
	docCount  int
	avgDocLen float64
	docLen    []int
	termFreqs []map[string]int
	idf       map[string]float64
	k1        float64
	b         float64
	DocIDs    []string
}

func NewBM25() *BM25 {
	return &BM25{
		k1: 1.2,
		b:  0.75,
	}
}

func (b *BM25) AddDocument(tokens map[string]int, docID string) {
	b.termFreqs = append(b.termFreqs, tokens)
	b.DocIDs = append(b.DocIDs, docID)
	var total int
	for _, count := range tokens {
		total += count
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

	df := make(map[string]int)
	for _, freqs := range b.termFreqs {
		for term := range freqs {
			df[term]++
		}
	}
	b.idf = make(map[string]float64)
	for term, count := range df {
		b.idf[term] = math.Log(1.0 + float64(b.docCount-count+1)/(float64(count)+0.5))
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
	results := make([]ScoredDoc, b.docCount)
	for i := 0; i < b.docCount; i++ {
		results[i] = ScoredDoc{Index: i, ID: b.DocIDs[i], Score: b.scoreDoc(i, query)}
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func (b *BM25) scoreDoc(docIdx int, query map[string]int) float64 {
	var score float64
	freqs := b.termFreqs[docIdx]
	docLen := float64(b.docLen[docIdx])

	for term := range query {
		if tf, ok := freqs[term]; ok {
			idf := b.idf[term]
			numer := float64(tf) * (b.k1 + 1)
			denom := float64(tf) + b.k1*(1-b.b+b.b*docLen/b.avgDocLen)
			score += idf * numer / denom
		}
	}
	return score
}

func (b *BM25) Search(query map[string]int, topK int) []ScoredDoc {
	scored := b.Score(query)
	if topK <= 0 || topK > len(scored) {
		topK = len(scored)
	}
	return scored[:topK]
}
