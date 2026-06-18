package engine

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/search"
	"github.com/robby031/smart-rag/pkg/storage"
)

const sampleUserTS = `
import { Database } from './db'
import { Logger } from '@company/logger'

export class UserService {
  constructor(private db: Database) {}

  async getUser(id: string): Promise<User> {
    return this.db.find(id)
  }

  async listUsers(): Promise<User[]> {
    const result = await this.getUser('all')
    return format(result)
  }
}

export function format(data: any): string {
  return JSON.stringify(data)
}

export enum UserStatus {
  Active = 'active',
  Inactive = 'inactive',
}

export type UserId = string
`

const sampleAuthTS = `
import { UserService } from './user'

export async function login(id: string) {
  const svc = new UserService()
  return svc.getUser(id)
}
`

func newJSTestEngine(t *testing.T) *Engine {
	t.Helper()

	dir := t.TempDir()
	kv, err := storage.OpenStore(filepath.Join(dir, "kv"))
	if err != nil {
		t.Fatalf("OpenStore: %v", err)
	}
	t.Cleanup(func() { kv.Close() })

	cs := storage.NewChunkStore(kv)
	gs := storage.NewGraphStore(kv)
	cg := graph.NewPersistentCallGraph(gs)
	ig := graph.NewPersistentImportGraph(gs)

	return &Engine{
		chunker:     indexer.NewChunker(512),
		parser:      indexer.NewParser(),
		tokenizer:   indexer.NewTokenizer(),
		bm25:        search.NewBM25(),
		graph:       graph.NewGraph(cg, ig),
		callGraph:   cg,
		importGraph: ig,
		chunkStore:  cs,
		pruningMode: PruningModeSoft,
	}
}

func TestJSIndexAndSearch(t *testing.T) {
	eng := newJSTestEngine(t)
	ctx := context.Background()

	if err := eng.IndexFile(ctx, "services/user.ts", sampleUserTS); err != nil {
		t.Fatalf("IndexFile user.ts: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	resp, err := eng.Query(ctx, Query{Type: QuerySearch, Text: "getUser async user service", TopK: 5})
	if err != nil {
		t.Fatalf("Query: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Fatal("expected search results for JS file, got none")
	}

	found := false
	for _, r := range resp.Results {
		if strings.Contains(r.Chunk.FilePath, "user.ts") {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected at least one result from user.ts")
	}
}

func TestJSFindDefinition(t *testing.T) {
	eng := newJSTestEngine(t)
	ctx := context.Background()

	if err := eng.IndexFile(ctx, "services/user.ts", sampleUserTS); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	cases := []struct {
		symbol    string
		wantLabel string
	}{
		{"UserService", "class"},
		{"UserStatus", "enum"},
		{"UserId", "type"},
		{"format", "func"},
	}
	for _, tc := range cases {
		resp, err := eng.Query(ctx, Query{Type: QueryDefinition, Text: tc.symbol})
		if err != nil {
			t.Fatalf("Query definition %q: %v", tc.symbol, err)
		}
		if len(resp.Results) == 0 {
			t.Errorf("no definition found for %q", tc.symbol)
			continue
		}
		found := false
		for _, r := range resp.Results {
			if strings.Contains(r.Content, "["+tc.wantLabel+"]") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("symbol %q: want label [%s] in results, got: %v", tc.symbol, tc.wantLabel, resp.Results)
		}
	}
}

func TestJSCallGraph(t *testing.T) {
	eng := newJSTestEngine(t)
	ctx := context.Background()

	if err := eng.IndexFile(ctx, "services/user.ts", sampleUserTS); err != nil {
		t.Fatalf("IndexFile: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	wantNodes := []string{
		"user.(UserService).getUser",
		"user.(UserService).listUsers",
		"user.format",
	}
	for _, id := range wantNodes {
		resp, err := eng.Query(ctx, Query{Type: QueryCallees, Text: id})
		if err != nil {
			t.Fatalf("QueryCallees %q: %v", id, err)
		}
		_ = resp
	}

	resp, err := eng.Query(ctx, Query{Type: QueryCallers, Text: "user.(UserService).getUser"})
	if err != nil {
		t.Fatalf("QueryCallers: %v", err)
	}
	found := false
	for _, r := range resp.Results {
		if strings.Contains(r.Content, "listUsers") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected listUsers as caller of getUser; got: %v", resp.Results)
	}
}

func TestJSImportGraph(t *testing.T) {
	eng := newJSTestEngine(t)
	ctx := context.Background()

	if err := eng.IndexFile(ctx, "services/user.ts", sampleUserTS); err != nil {
		t.Fatalf("IndexFile user.ts: %v", err)
	}
	if err := eng.IndexFile(ctx, "services/auth.ts", sampleAuthTS); err != nil {
		t.Fatalf("IndexFile auth.ts: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	deps := eng.importGraph.Dependencies("user")
	depsMap := make(map[string]bool, len(deps))
	for _, d := range deps {
		depsMap[d] = true
	}
	if !depsMap["./db"] {
		t.Errorf("user.ts should depend on ./db; got deps: %v", deps)
	}
	if !depsMap["@company/logger"] {
		t.Errorf("user.ts should depend on @company/logger; got deps: %v", deps)
	}

	authDeps := eng.importGraph.Dependencies("auth")
	authMap := make(map[string]bool, len(authDeps))
	for _, d := range authDeps {
		authMap[d] = true
	}
	if !authMap["./user"] {
		t.Errorf("auth.ts should depend on ./user; got deps: %v", authDeps)
	}

	dependents := eng.importGraph.Dependents("./user")
	depSet := make(map[string]bool, len(dependents))
	for _, d := range dependents {
		depSet[d] = true
	}
	if !depSet["auth"] {
		t.Errorf("./user should have auth as dependent; got: %v", dependents)
	}

	_ = ctx
}

func TestJSImpactAnalysis(t *testing.T) {
	eng := newJSTestEngine(t)
	ctx := context.Background()

	if err := eng.IndexFile(ctx, "services/user.ts", sampleUserTS); err != nil {
		t.Fatalf("IndexFile user.ts: %v", err)
	}
	if err := eng.IndexFile(ctx, "services/auth.ts", sampleAuthTS); err != nil {
		t.Fatalf("IndexFile auth.ts: %v", err)
	}
	if err := eng.FinalizeIndex(); err != nil {
		t.Fatalf("FinalizeIndex: %v", err)
	}

	resp, err := eng.Query(ctx, Query{
		Type:     QueryImpact,
		Text:     "user.(UserService).getUser",
		MaxDepth: 3,
	})
	if err != nil {
		t.Fatalf("QueryImpact: %v", err)
	}
	if len(resp.Results) == 0 {
		t.Error("expected impact results for getUser, got none")
	}
}
