package engine

import (
	"context"
	"strings"
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

	// Over-fetch when filters are active so post-filter count approaches topK.
	fetchK := topK
	if q.Language != "" || q.File != "" {
		fetchK = topK * 5
	}

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
		resp.Results = append(resp.Results, Result{Score: sr.Score, Chunk: chunk, Content: chunk.Content})
		if len(resp.Results) >= topK {
			break
		}
	}

	return resp, nil
}
