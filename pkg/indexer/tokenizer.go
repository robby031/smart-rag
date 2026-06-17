package indexer

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

type SparseVector map[string]float64

type Tokenizer struct {
	stopwords map[string]bool
}

func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		stopwords: defaultStopwords(),
	}
}

func (t *Tokenizer) Tokenize(code string) map[string]int {
	raw := tokenizeCode(code)
	freq := make(map[string]int)
	for _, tok := range raw {
		tok = strings.ToLower(tok)
		if t.stopwords[tok] {
			continue
		}
		if len(tok) <= 1 {
			continue
		}
		freq[tok]++

		subtokens := splitCamel(tok)
		for _, st := range subtokens {
			if len(st) > 1 && !t.stopwords[st] {
				freq[st]++
			}
		}
	}
	return freq
}

func (t *Tokenizer) TFIDF(tf map[string]int, idf map[string]float64) SparseVector {
	vec := make(SparseVector)
	for term, count := range tf {
		weight := float64(count)
		if df, ok := idf[term]; ok && df > 0 {
			weight *= math.Log(1.0 + float64(len(idf))/df)
		}
		vec[term] = weight
	}
	return vec
}

func IDF(collection []map[string]int) map[string]float64 {
	df := make(map[string]int)
	for _, doc := range collection {
		for term := range doc {
			df[term]++
		}
	}
	idf := make(map[string]float64)
	for term, count := range df {
		idf[term] = float64(count)
	}
	return idf
}

func CosineSimilarity(a, b SparseVector) float64 {
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

func (v SparseVector) TopN(n int) []string {
	type kv struct {
		k string
		v float64
	}
	var sorted []kv
	for k, val := range v {
		sorted = append(sorted, kv{k, val})
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].v > sorted[j].v
	})
	if n > len(sorted) {
		n = len(sorted)
	}
	result := make([]string, n)
	for i := 0; i < n; i++ {
		result[i] = sorted[i].k
	}
	return result
}

func tokenizeCode(code string) []string {
	var tokens []string
	var buf strings.Builder

	flush := func() {
		if buf.Len() > 0 {
			tokens = append(tokens, buf.String())
			buf.Reset()
		}
	}

	for _, r := range code {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			flush()
		} else if r == '(' || r == ')' || r == '{' || r == '}' || r == '[' || r == ']' ||
			r == ';' || r == ',' || r == '.' || r == ':' {
			flush()
			tokens = append(tokens, string(r))
		} else {
			buf.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func splitCamel(s string) []string {
	var parts []string
	start := 0
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			if i-start > 0 {
				parts = append(parts, s[start:i])
			}
			start = i
		}
	}
	if start < len(s) {
		parts = append(parts, s[start:])
	}
	// Remove duplicates
	seen := make(map[string]bool)
	uniq := parts[:0]
	for _, p := range parts {
		if !seen[p] {
			seen[p] = true
			uniq = append(uniq, p)
		}
	}
	return uniq
}

func defaultStopwords() map[string]bool {
	return map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "it": true,
		"of": true, "in": true, "to": true, "for": true, "on": true,
		"and": true, "or": true, "not": true, "with": true, "as": true,
		"by": true, "at": true, "from": true, "be": true, "this": true,
		"that": true, "if": true, "else": true, "return": true,
		"nil": true, "true": true, "false": true, "int": true,
		"string": true, "bool": true, "error": true, "byte": true,
		"uint": true, "int8": true, "int16": true, "int32": true,
		"int64": true, "float32": true, "float64": true,
		"package": true, "import": true, "func": true, "type": true,
		"struct": true, "interface": true, "var": true, "const": true,
		"map": true, "chan": true, "go": true, "defer": true,
		"select": true, "case": true, "default": true, "switch": true,
		"range": true, "break": true, "continue": true, "goto": true,
		"fallthrough": true, "append": true, "len": true, "cap": true,
		"make": true, "new": true, "panic": true, "recover": true,
		"close": true, "uintptr": true, "complex64": true, "complex128": true,
		"uint8": true, "uint16": true, "uint32": true, "uint64": true,
	}
}
