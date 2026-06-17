package search

import (
	"math"
	"sort"
	"sync"
)

type posting struct {
	docIdx int32
	tf     int32
}

type BM25 struct {
	mu          sync.Mutex
	docCount    int
	avgDocLen   float64
	docLen      []int32
	idf         map[string]float64
	k1          float64
	b           float64
	DocIDs      []string
	lenNorm     []float64
	termPosting map[string][]posting
}

func NewBM25() *BM25 {
	return &BM25{
		k1:          1.2,
		b:           0.75,
		termPosting: make(map[string][]posting),
	}
}

type LocalBM25 struct {
	docLen      []int32
	DocIDs      []string
	termPosting map[string][]posting
}

func NewLocalBM25() *LocalBM25 {
	return &LocalBM25{termPosting: make(map[string][]posting)}
}

func (l *LocalBM25) AddDocument(tokens map[string]int, docID string) {
	idx := int32(len(l.DocIDs))
	l.DocIDs = append(l.DocIDs, docID)
	var total int32
	for term, count := range tokens {
		total += int32(count)
		l.termPosting[term] = append(l.termPosting[term], posting{docIdx: idx, tf: int32(count)})
	}
	l.docLen = append(l.docLen, total)
}

func (b *BM25) Merge(locals []*LocalBM25) {
	capacities := make(map[string]int, len(b.termPosting))
	var totalDocs int
	for _, local := range locals {
		totalDocs += len(local.DocIDs)
		for term, posts := range local.termPosting {
			capacities[term] += len(posts)
		}
	}

	// Pre-allocate exact-size slices — zero wasted capacity after merge.
	for term, n := range capacities {
		b.termPosting[term] = make([]posting, 0, n)
	}
	b.DocIDs = make([]string, 0, totalDocs)
	b.docLen = make([]int32, 0, totalDocs)

	var offset int32
	for _, local := range locals {
		for term, posts := range local.termPosting {
			for i := range posts {
				posts[i].docIdx += offset
			}
			b.termPosting[term] = append(b.termPosting[term], posts...)
		}
		b.DocIDs = append(b.DocIDs, local.DocIDs...)
		b.docLen = append(b.docLen, local.docLen...)
		offset += int32(len(local.DocIDs))
	}
	b.docCount = int(offset)
}

func (b *BM25) AddDocument(tokens map[string]int, docID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	idx := int32(b.docCount)
	b.DocIDs = append(b.DocIDs, docID)
	var total int32
	for term, count := range tokens {
		total += int32(count)
		b.termPosting[term] = append(b.termPosting[term], posting{docIdx: idx, tf: int32(count)})
	}
	b.docLen = append(b.docLen, total)
	b.docCount++
}

func (b *BM25) Build() {
	if b.docCount == 0 {
		return
	}
	var totalLen int64
	for _, l := range b.docLen {
		totalLen += int64(l)
	}
	b.avgDocLen = float64(totalLen) / float64(b.docCount)

	b.idf = make(map[string]float64, len(b.termPosting))
	for term, posts := range b.termPosting {
		df := len(posts)
		b.idf[term] = math.Log(1.0 + float64(b.docCount-df+1)/(float64(df)+0.5))
	}

	b.lenNorm = make([]float64, b.docCount)
	for i, l := range b.docLen {
		b.lenNorm[i] = 1 - b.b + b.b*float64(l)/b.avgDocLen
	}
	b.docLen = nil // free memory after build

	pruneThreshold := b.docCount * 4 / 5
	for term, posts := range b.termPosting {
		if len(posts) > pruneThreshold {
			delete(b.termPosting, term)
			delete(b.idf, term)
		}
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

	candidates := make(map[int32]float64)
	for term := range query {
		idf := b.idf[term]
		if idf == 0 {
			continue
		}
		for _, p := range b.termPosting[term] {
			tf := float64(p.tf)
			numer := tf * (b.k1 + 1)
			denom := tf + b.k1*b.lenNorm[p.docIdx]
			candidates[p.docIdx] += idf * numer / denom
		}
	}

	if len(candidates) == 0 {
		return nil
	}

	results := make([]ScoredDoc, 0, len(candidates))
	for idx, score := range candidates {
		results = append(results, ScoredDoc{Index: int(idx), ID: b.DocIDs[idx], Score: score})
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})
	return results
}

func (b *BM25) Search(query map[string]int, topK int) []ScoredDoc {
	scored := b.Score(query)
	if len(scored) <= topK {
		return scored
	}
	return scored[:topK]
}
