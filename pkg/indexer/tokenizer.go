package indexer

import (
	"math"
	"sort"
	"strings"
	"unicode"
)

type SparseVector map[string]float64

type Tokenizer struct {
	codeStopwords  map[string]bool
	queryStopwords map[string]bool
}

func NewTokenizer() *Tokenizer {
	return &Tokenizer{
		codeStopwords:  defaultCodeStopwords(),
		queryStopwords: defaultQueryStopwords(),
	}
}

func (t *Tokenizer) Tokenize(code string) map[string]int {
	return t.tokenize(code, t.codeStopwords)
}

func (t *Tokenizer) TokenizeQuery(query string) map[string]int {
	return t.tokenize(query, t.queryStopwords)
}

func (t *Tokenizer) tokenize(text string, stopwords map[string]bool) map[string]int {
	raw := tokenizeSearchText(text)
	freq := make(map[string]int)
	for _, tok := range raw {
		terms := expandSearchToken(tok)
		for _, term := range terms {
			if stopwords[term] {
				continue
			}
			if len(term) <= 1 {
				continue
			}
			freq[term]++
		}
	}
	return freq
}

func addUniqueTerm(terms *[]string, seen map[string]bool, term string) {
	term = strings.ToLower(strings.Trim(term, "._-/\\"))
	if len(term) <= 1 {
		return
	}
	if seen[term] {
		return
	}
	seen[term] = true
	*terms = append(*terms, term)
}

func expandSearchToken(tok string) []string {
	tok = strings.Trim(tok, "._-/\\")
	if tok == "" {
		return nil
	}

	var terms []string
	seen := make(map[string]bool)
	addUniqueTerm(&terms, seen, tok)

	for _, part := range strings.FieldsFunc(tok, isSearchSeparator) {
		addUniqueTerm(&terms, seen, part)
		for _, sub := range splitIdentifier(part) {
			addUniqueTerm(&terms, seen, sub)
		}
	}

	if !strings.ContainsFunc(tok, isSearchSeparator) {
		for _, sub := range splitIdentifier(tok) {
			addUniqueTerm(&terms, seen, sub)
		}
	}

	return terms
}

func tokenizeSearchText(text string) []string {
	var tokens []string
	var buf strings.Builder

	flush := func() {
		if buf.Len() == 0 {
			return
		}
		tokens = append(tokens, buf.String())
		buf.Reset()
	}

	for _, r := range text {
		if isSearchTokenRune(r) {
			buf.WriteRune(r)
			continue
		}
		flush()
	}
	flush()
	return tokens
}

func isSearchTokenRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || isSearchSeparator(r)
}

func isSearchSeparator(r rune) bool {
	switch r {
	case '_', '-', '.', '/', '\\':
		return true
	default:
		return false
	}
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
		switch r {
		case ' ', '\t', '\n', '\r':
			flush()
		case '(', ')', '{', '}', '[', ']', ';', ',', '.', ':':
			flush()
			tokens = append(tokens, string(r))
		default:
			buf.WriteRune(r)
		}
	}
	flush()
	return tokens
}

func splitCamel(s string) []string {
	return splitIdentifier(s)
}

func splitIdentifier(s string) []string {
	runes := []rune(s)
	if len(runes) == 0 {
		return nil
	}

	var parts []string
	start := 0
	for i := 1; i < len(runes); i++ {
		prev := runes[i-1]
		cur := runes[i]
		var next rune
		hasNext := i+1 < len(runes)
		if hasNext {
			next = runes[i+1]
		}

		if isIdentifierBoundary(prev, cur, next, hasNext) {
			parts = append(parts, string(runes[start:i]))
			start = i
		}
	}
	if start < len(runes) {
		parts = append(parts, string(runes[start:]))
	}

	seen := make(map[string]bool)
	uniq := parts[:0]
	for _, p := range parts {
		p = strings.ToLower(p)
		if len(p) <= 1 || seen[p] {
			continue
		}
		seen[p] = true
		uniq = append(uniq, p)
	}
	return uniq
}

func isIdentifierBoundary(prev, cur, next rune, hasNext bool) bool {
	if unicode.IsDigit(prev) != unicode.IsDigit(cur) {
		return true
	}
	if unicode.IsLower(prev) && unicode.IsUpper(cur) {
		return true
	}
	if unicode.IsUpper(prev) && unicode.IsUpper(cur) && hasNext && unicode.IsLower(next) {
		return true
	}
	return false
}

func defaultCodeStopwords() map[string]bool {
	return map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "it": true,
		"of": true, "in": true, "to": true, "for": true, "on": true,
		"and": true, "or": true, "not": true, "with": true, "as": true,
		"by": true, "at": true, "from": true, "be": true, "this": true,
		"that": true, "if": true, "else": true, "return": true,
		"package": true, "import": true, "func": true, "type": true,
		"struct": true, "interface": true, "var": true, "const": true,
		"chan": true, "defer": true,
		"select": true, "case": true, "default": true, "switch": true,
		"range": true, "break": true, "continue": true, "goto": true,
		"fallthrough": true,
	}
}

func defaultQueryStopwords() map[string]bool {
	return map[string]bool{
		"a": true, "an": true, "the": true, "is": true, "it": true,
		"of": true, "in": true, "to": true, "for": true, "on": true,
		"and": true, "or": true, "not": true, "with": true, "as": true,
		"by": true, "at": true, "from": true, "be": true, "this": true,
		"that": true,
	}
}
