package indexer

import "testing"

func TestTokenizerSplitsIdentifiersBeforeLowercase(t *testing.T) {
	tokens := NewTokenizer().Tokenize("func GetContextPack() { return HTTPServerID }")

	for _, term := range []string{"getcontextpack", "get", "context", "pack", "http", "server", "id"} {
		if tokens[term] == 0 {
			t.Fatalf("expected token %q in %v", term, tokens)
		}
	}
}

func TestTokenizerSupportsSeparatedSymbolsAndPaths(t *testing.T) {
	tokens := NewTokenizer().Tokenize("pkg/engine/context.go handle_get-context.pack engine.New")

	for _, term := range []string{
		"pkg/engine/context.go",
		"pkg",
		"engine",
		"context",
		"go",
		"handle_get-context.pack",
		"handle",
		"get",
		"pack",
		"engine.new",
		"new",
	} {
		if tokens[term] == 0 {
			t.Fatalf("expected token %q in %v", term, tokens)
		}
	}
}

func TestTokenizerKeepsCodeMeaningfulTerms(t *testing.T) {
	tokens := NewTokenizer().Tokenize("func readStringMap() (map[string]error, bool) { return nil, false }")

	for _, term := range []string{"string", "map", "error", "bool", "nil", "false"} {
		if tokens[term] == 0 {
			t.Fatalf("expected code-meaningful token %q in %v", term, tokens)
		}
	}
}

func TestTokenizerQueryUsesConservativeNaturalStopwords(t *testing.T) {
	tokens := NewTokenizer().TokenizeQuery("find the get_context_pack handler in pkg/engine/context.go")

	if tokens["the"] != 0 || tokens["in"] != 0 {
		t.Fatalf("expected natural stopwords to be removed from query tokens: %v", tokens)
	}
	for _, term := range []string{"find", "get_context_pack", "get", "context", "pack", "handler", "pkg", "engine"} {
		if tokens[term] == 0 {
			t.Fatalf("expected query token %q in %v", term, tokens)
		}
	}
}
