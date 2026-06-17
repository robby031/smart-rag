package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/storage"
)

func TestFinalizeIndexRefreshesChunkReachability(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package main",
		"",
		"func main() { live() }",
		"",
		"func live() {",
		"	prepare()",
		"	helper()",
		"}",
		"",
		"func prepare() {}",
		"",
		"func helper() {}",
		"",
		"func dead() {}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "app/main.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	live := mustChunkBySymbol(t, eng.chunkStore, "live")
	if live.Reachability != ReachabilityReachable {
		t.Fatalf("live reachability mismatch: %+v", live)
	}
	if live.ContextWeight != reachableContextWeight {
		t.Fatalf("live context weight mismatch: %.4f", live.ContextWeight)
	}

	dead := mustChunkBySymbol(t, eng.chunkStore, "dead")
	if dead.Reachability != ReachabilityUnreachable {
		t.Fatalf("dead reachability mismatch: %+v", dead)
	}
	if dead.ContextWeight != unreachableContextWeight {
		t.Fatalf("dead context weight mismatch: %.4f", dead.ContextWeight)
	}
}

func TestSearchAppliesReachabilityPenaltyWithoutDroppingResults(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/live.go:1-3",
			FilePath:      "pkg/live.go",
			ChunkType:     "4",
			SymbolName:    "Live",
			StartLine:     1,
			EndLine:       3,
			Content:       "func Live() { sharedPruningToken() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:            "pkg/dead.go:1-3",
			FilePath:      "pkg/dead.go",
			ChunkType:     "4",
			SymbolName:    "dead",
			StartLine:     1,
			EndLine:       3,
			Content:       "func dead() { sharedPruningToken() }",
			Reachability:  ReachabilityUnreachable,
			ContextWeight: unreachableContextWeight,
		},
		{
			ID:         "pkg/other.go:1-3",
			FilePath:   "pkg/other.go",
			ChunkType:  "4",
			SymbolName: "Other",
			StartLine:  1,
			EndLine:    3,
			Content:    "func Other() { unrelatedToken() }",
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "sharedPruningToken",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected both reachable and unreachable results, got %d", len(resp.Results))
	}
	if got, want := resp.Results[0].Chunk.ID, "pkg/live.go:1-3"; got != want {
		t.Fatalf("reachable chunk should rank first: want %s, got %s", want, got)
	}
	if got, want := resp.Results[1].Chunk.ID, "pkg/dead.go:1-3"; got != want {
		t.Fatalf("unreachable chunk should still be returned: want %s, got %s", want, got)
	}
	if !strings.Contains(strings.Join(resp.Results[1].Related, "\n"), "penalty reachability=unreachable") {
		t.Fatalf("expected reachability penalty explanation, got %v", resp.Results[1].Related)
	}
}

func TestContextPackSkipsUnreachableNearbyChunks(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/service.go:1-3",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "deadNearby",
			StartLine:     1,
			EndLine:       3,
			Content:       "func deadNearby() { lowValueBoilerplate() }",
			Reachability:  ReachabilityUnreachable,
			ContextWeight: unreachableContextWeight,
		},
		{
			ID:            "pkg/service.go:5-8",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "Target",
			StartLine:     5,
			EndLine:       8,
			Content:       "func Target() { importantLogic() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:            "pkg/service.go:10-12",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "liveNearby",
			StartLine:     10,
			EndLine:       12,
			Content:       "func liveNearby() { usefulFollowup() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QueryContextPack,
		Text: "pkg/service.go:5-8",
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}
	content := resp.Results[0].Content
	if strings.Contains(content, "deadNearby") {
		t.Fatalf("unreachable nearby chunk leaked into context pack:\n%s", content)
	}
	if !strings.Contains(content, "liveNearby") {
		t.Fatalf("reachable nearby chunk missing from context pack:\n%s", content)
	}
}

func TestEntrypointReachabilityIncludesHTTPHandlers(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package server",
		"",
		"import \"net/http\"",
		"",
		"func handleLogin(w http.ResponseWriter, r *http.Request) { validateLogin() }",
		"",
		"func validateLogin() {}",
		"",
		"func unusedHelper() {}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "internal/server/routes.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	if got := mustChunkBySymbol(t, eng.chunkStore, "handleLogin").Reachability; got != ReachabilityReachable {
		t.Fatalf("HTTP handler root reachability mismatch: %s", got)
	}
	if got := mustChunkBySymbol(t, eng.chunkStore, "validateLogin").Reachability; got != ReachabilityReachable {
		t.Fatalf("HTTP handler callee reachability mismatch: %s", got)
	}
	if got := mustChunkBySymbol(t, eng.chunkStore, "unusedHelper").Reachability; got != ReachabilityUnreachable {
		t.Fatalf("unused helper reachability mismatch: %s", got)
	}
}

func TestEntrypointReachabilityIncludesCLICommands(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package cli",
		"",
		"func runRoot() { executeUsecase() }",
		"",
		"func executeUsecase() {}",
		"",
		"func unusedJob() {}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "internal/cli/root.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	if got := mustChunkBySymbol(t, eng.chunkStore, "runRoot").Reachability; got != ReachabilityReachable {
		t.Fatalf("CLI command root reachability mismatch: %s", got)
	}
	if got := mustChunkBySymbol(t, eng.chunkStore, "executeUsecase").Reachability; got != ReachabilityReachable {
		t.Fatalf("CLI command callee reachability mismatch: %s", got)
	}
	if got := mustChunkBySymbol(t, eng.chunkStore, "unusedJob").Reachability; got != ReachabilityUnreachable {
		t.Fatalf("unused job reachability mismatch: %s", got)
	}
}

func TestEntrypointReachabilityMarksExportedTypesReachable(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package domain",
		"",
		"type PublicConfig struct {",
		"	Value string",
		"}",
		"",
		"type privateConfig struct {",
		"	Value string",
		"}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "internal/domain/config.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	if got := mustChunkBySymbol(t, eng.chunkStore, "PublicConfig").Reachability; got != ReachabilityReachable {
		t.Fatalf("exported type reachability mismatch: %s", got)
	}
	if got := mustChunkBySymbol(t, eng.chunkStore, "privateConfig").Reachability; got != ReachabilityUnknown {
		t.Fatalf("private type reachability mismatch: %s", got)
	}
}

func TestSearchSymbolMatchActsAsQueryReachabilityRoot(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package main",
		"",
		"func main() {}",
		"",
		"func deadRoot() { deadLeaf() }",
		"",
		"func deadLeaf() {}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "app/main.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}
	if got := mustChunkBySymbol(t, eng.chunkStore, "deadRoot").Reachability; got != ReachabilityUnreachable {
		t.Fatalf("baseline deadRoot reachability mismatch: %s", got)
	}

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "deadRoot",
		TopK: 3,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected search results")
	}
	if got, want := resp.Results[0].Chunk.SymbolName, "deadRoot"; got != want {
		t.Fatalf("expected query root result first: want %s, got %s", want, got)
	}
	details := strings.Join(resp.Results[0].Related, "\n")
	if strings.Contains(details, "penalty reachability=unreachable") {
		t.Fatalf("query root should skip unreachable penalty, got details: %v", resp.Results[0].Related)
	}
	if !strings.Contains(details, "query_reachable_root=skip_unreachable_penalty") {
		t.Fatalf("expected query reachability explanation, got %v", resp.Results[0].Related)
	}
}

func TestSemanticFoldingMarksBoilerplateKinds(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"package app",
		"",
		"type UserResponse struct {",
		"	ID string `json:\"id\"`",
		"	Name string `json:\"name\"`",
		"	Email string `json:\"email\"`",
		"	Phone string `json:\"phone\"`",
		"	Address string `json:\"address\"`",
		"	City string `json:\"city\"`",
		"	Country string `json:\"country\"`",
		"	Status string `json:\"status\"`",
		"}",
		"",
		"const (",
		"	ErrNotFound = \"not_found\"",
		"	ErrInvalid = \"invalid\"",
		")",
		"",
		"func NewUserResponse() *UserResponse {",
		"	return &UserResponse{}",
		"}",
		"",
		"func (u *UserResponse) GetName() string {",
		"	return u.Name",
		"}",
		"",
		"func SaveUser() error {",
		"	return saveUser()",
		"}",
		"",
		"func importantLogic() {",
		"	validateState()",
		"	persistState()",
		"}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "internal/app/user.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	assertFoldReason(t, eng.chunkStore, "UserResponse", FoldReasonLargeDTO)
	assertFoldReason(t, eng.chunkStore, "ErrNotFound", FoldReasonErrorConstBlock)
	assertFoldReason(t, eng.chunkStore, "NewUserResponse", FoldReasonTrivialConstructor)
	assertFoldReason(t, eng.chunkStore, "GetName", FoldReasonGetterSetter)
	assertFoldReason(t, eng.chunkStore, "SaveUser", FoldReasonSimpleWrapper)

	important := mustChunkBySymbol(t, eng.chunkStore, "importantLogic")
	if important.FoldReason != "" || important.SemanticRole != "" {
		t.Fatalf("important logic should not be folded: %+v", important)
	}
}

func TestSemanticFoldingMarksGeneratedCode(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()

	src := strings.Join([]string{
		"// Code generated by protoc-gen-go. DO NOT EDIT.",
		"package gen",
		"",
		"func GeneratedHelper() {}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "internal/gen/pb.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	chunk := assertFoldReason(t, eng.chunkStore, "GeneratedHelper", FoldReasonGeneratedCode)
	if chunk.ContextWeight != generatedContextWeight {
		t.Fatalf("generated code weight mismatch: %.4f", chunk.ContextWeight)
	}
}

func TestSearchAppliesSemanticFoldingPenaltyWithoutDroppingResults(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/logic.go:1-5",
			FilePath:      "pkg/logic.go",
			ChunkType:     "4",
			SymbolName:    "ProcessPayment",
			StartLine:     1,
			EndLine:       5,
			Content:       "func ProcessPayment() { sharedSemanticToken(); validateRisk() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:            "pkg/wrapper.go:1-3",
			FilePath:      "pkg/wrapper.go",
			ChunkType:     "4",
			SymbolName:    "WrapPayment",
			StartLine:     1,
			EndLine:       3,
			Content:       "func WrapPayment() { sharedSemanticToken() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: boilerplateContextWeight,
			SemanticRole:  SemanticRoleBoilerplate,
			FoldReason:    FoldReasonSimpleWrapper,
		},
		{
			ID:         "pkg/other.go:1-3",
			FilePath:   "pkg/other.go",
			ChunkType:  "4",
			SymbolName: "Other",
			StartLine:  1,
			EndLine:    3,
			Content:    "func Other() { unrelatedToken() }",
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "sharedSemanticToken",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected both semantic results, got %d", len(resp.Results))
	}
	if got, want := resp.Results[0].Chunk.ID, "pkg/logic.go:1-5"; got != want {
		t.Fatalf("logic chunk should rank first: want %s, got %s", want, got)
	}
	if got, want := resp.Results[1].Chunk.ID, "pkg/wrapper.go:1-3"; got != want {
		t.Fatalf("folded wrapper should still be returned: want %s, got %s", want, got)
	}
	if !strings.Contains(strings.Join(resp.Results[1].Related, "\n"), "penalty semantic_role=boilerplate fold_reason=simple_wrapper") {
		t.Fatalf("expected semantic folding penalty explanation, got %v", resp.Results[1].Related)
	}
}

func TestContextPackSkipsFoldedNearbyChunks(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/service.go:1-3",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "NewService",
			StartLine:     1,
			EndLine:       3,
			Content:       "func NewService() *Service { return &Service{} }",
			Reachability:  ReachabilityReachable,
			ContextWeight: boilerplateContextWeight,
			SemanticRole:  SemanticRoleBoilerplate,
			FoldReason:    FoldReasonTrivialConstructor,
		},
		{
			ID:            "pkg/service.go:5-8",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "Target",
			StartLine:     5,
			EndLine:       8,
			Content:       "func Target() { importantLogic() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:            "pkg/service.go:10-12",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "processDomain",
			StartLine:     10,
			EndLine:       12,
			Content:       "func processDomain() { usefulFollowup() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
	})

	resp, err := eng.Query(context.Background(), Query{
		Type: QueryContextPack,
		Text: "pkg/service.go:5-8",
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}
	content := resp.Results[0].Content
	if strings.Contains(content, "NewService") {
		t.Fatalf("folded nearby chunk leaked into context pack:\n%s", content)
	}
	if !strings.Contains(content, "processDomain") {
		t.Fatalf("normal nearby chunk missing from context pack:\n%s", content)
	}
}

func TestParsePruningMode(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want PruningMode
	}{
		{in: "", want: PruningModeSoft},
		{in: "soft", want: PruningModeSoft},
		{in: "off", want: PruningModeOff},
		{in: "hard", want: PruningModeHard},
		{in: " HARD ", want: PruningModeHard},
	} {
		got, err := ParsePruningMode(tc.in)
		if err != nil {
			t.Fatalf("ParsePruningMode(%q): %v", tc.in, err)
		}
		if got != tc.want {
			t.Fatalf("ParsePruningMode(%q): want %s, got %s", tc.in, tc.want, got)
		}
	}
	if _, err := ParsePruningMode("delete-everything"); err == nil {
		t.Fatal("expected invalid pruning mode to fail")
	}
}

func TestPruningModeOffIgnoresStoredPruningMetadata(t *testing.T) {
	eng := regressionTestEngineWithChunks(t, []storage.ChunkMeta{
		{
			ID:            "pkg/service.go:1-3",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "FoldedNearby",
			StartLine:     1,
			EndLine:       3,
			Content:       "func FoldedNearby() { sharedModeToken() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: boilerplateContextWeight,
			SemanticRole:  SemanticRoleBoilerplate,
			FoldReason:    FoldReasonSimpleWrapper,
		},
		{
			ID:            "pkg/service.go:5-8",
			FilePath:      "pkg/service.go",
			ChunkType:     "4",
			SymbolName:    "Target",
			StartLine:     5,
			EndLine:       8,
			Content:       "func Target() { sharedModeToken() }",
			Reachability:  ReachabilityReachable,
			ContextWeight: reachableContextWeight,
		},
		{
			ID:         "pkg/other.go:1-3",
			FilePath:   "pkg/other.go",
			ChunkType:  "4",
			SymbolName: "Other",
			StartLine:  1,
			EndLine:    3,
			Content:    "func Other() { unrelatedToken() }",
		},
	})
	if err := eng.SetPruningMode(PruningModeOff); err != nil {
		t.Fatalf("SetPruningMode: %v", err)
	}

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "sharedModeToken",
		TopK: 2,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) != 2 {
		t.Fatalf("expected two search results, got %d", len(resp.Results))
	}
	if strings.Contains(strings.Join(resp.Results[0].Related, "\n"), "penalty semantic_role=") ||
		strings.Contains(strings.Join(resp.Results[1].Related, "\n"), "penalty semantic_role=") {
		t.Fatalf("pruning off should ignore semantic penalty: %+v", resp.Results)
	}

	ctxResp, err := eng.Query(context.Background(), Query{
		Type: QueryContextPack,
		Text: "pkg/service.go:5-8",
	})
	if err != nil {
		t.Fatalf("Query context pack: %v", err)
	}
	if !strings.Contains(ctxResp.Results[0].Content, "FoldedNearby") {
		t.Fatalf("pruning off should include folded nearby chunks:\n%s", ctxResp.Results[0].Content)
	}
}

func TestHardPruningDeletesLowValueChunksAndRebuildsBM25(t *testing.T) {
	eng, closeStore := pruningTestEngine(t)
	defer closeStore()
	if err := eng.SetPruningMode(PruningModeHard); err != nil {
		t.Fatalf("SetPruningMode: %v", err)
	}

	src := strings.Join([]string{
		"package main",
		"",
		"func main() { importantLogic() }",
		"",
		"func importantLogic() {",
		"	validatePayment()",
		"	persistPayment()",
		"}",
		"",
		"func deadHelper() { hardPruneDeadToken() }",
		"",
		"func NewService() *Service {",
		"	return &Service{}",
		"}",
		"",
		"type Service struct {",
		"	Name string",
		"}",
	}, "\n")

	if err := eng.IndexFile(context.Background(), "app/main.go", src); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	if chunk := findChunkBySymbol(t, eng.chunkStore, "importantLogic"); chunk == nil {
		t.Fatal("important logic should remain after hard pruning")
	}
	if chunk := findChunkBySymbol(t, eng.chunkStore, "deadHelper"); chunk != nil {
		t.Fatalf("unreachable function should be hard pruned: %+v", chunk)
	}
	if chunk := findChunkBySymbol(t, eng.chunkStore, "NewService"); chunk != nil {
		t.Fatalf("trivial constructor should be hard pruned: %+v", chunk)
	}

	resp, err := eng.Query(context.Background(), Query{
		Type: QuerySearch,
		Text: "hardPruneDeadToken",
		TopK: 5,
	})
	if err != nil {
		t.Fatalf("Query search: %v", err)
	}
	if len(resp.Results) != 0 {
		t.Fatalf("hard-pruned chunk leaked through rebuilt BM25: %+v", resp.Results)
	}
}

func pruningTestEngine(t *testing.T) (*Engine, func()) {
	t.Helper()

	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	return New(kv, storage.NewChunkStore(kv), nil, nil), func() {
		if err := kv.Close(); err != nil {
			t.Fatalf("Close store: %v", err)
		}
	}
}

func findChunkBySymbol(t *testing.T, store *storage.ChunkStore, symbol string) *storage.ChunkMeta {
	t.Helper()

	chunks, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	for _, chunk := range chunks {
		if chunk.SymbolName == symbol {
			return chunk
		}
	}
	return nil
}

func assertFoldReason(t *testing.T, store *storage.ChunkStore, symbol, reason string) *storage.ChunkMeta {
	t.Helper()

	chunk := mustChunkBySymbol(t, store, symbol)
	if chunk.SemanticRole != SemanticRoleBoilerplate {
		t.Fatalf("%s semantic role mismatch: %+v", symbol, chunk)
	}
	if chunk.FoldReason != reason {
		t.Fatalf("%s fold reason mismatch: want %s, got %s", symbol, reason, chunk.FoldReason)
	}
	if chunk.ContextWeight > boilerplateContextWeight && reason != FoldReasonGeneratedCode {
		t.Fatalf("%s context weight was not folded: %.4f", symbol, chunk.ContextWeight)
	}
	return chunk
}

func mustChunkBySymbol(t *testing.T, store *storage.ChunkStore, symbol string) *storage.ChunkMeta {
	t.Helper()

	chunks, err := store.GetAll()
	if err != nil {
		t.Fatalf("GetAll: %v", err)
	}
	for _, chunk := range chunks {
		if chunk.SymbolName == symbol {
			return chunk
		}
	}
	t.Fatalf("chunk with symbol %q not found", symbol)
	return nil
}
