package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/robby031/smart-rag/pkg/engine"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/searcher"
	"github.com/robby031/smart-rag/pkg/storage"
)

var version = "dev"

func main() {
	repoDir := flag.String("repo", ".", "Path to the code repository")
	dbDir := flag.String("db", "", "Path to the RAG database (default: temp dir)")
	pruningMode := flag.String("pruning", string(engine.PruningModeSoft), "Index pruning mode: off, soft, or hard")
	flag.Parse()

	absRepo, _ := filepath.Abs(*repoDir)

	absDB := *dbDir
	if absDB == "" {
		tmp, err := os.MkdirTemp("", "rag-bench-*")
		if err != nil {
			log.Fatal(err)
		}
		defer os.RemoveAll(tmp)
		absDB = tmp
	} else {
		absDB, _ = filepath.Abs(absDB)
		os.MkdirAll(absDB, 0755)
	}

	totalFiles, totalLines := repoStats(absRepo)
	if totalFiles == 0 {
		log.Fatalf("No indexable files found in %s", absRepo)
	}

	kvStore, err := storage.OpenStore(filepath.Join(absDB, "kv"))
	if err != nil {
		log.Fatal(err)
	}
	defer kvStore.Close()

	chunkStore := storage.NewChunkStore(kvStore)
	graphStore := storage.NewGraphStore(kvStore)
	indexStore := storage.NewIndexStore(kvStore)

	vectorDB, err := storage.NewVectorDB(filepath.Join(absDB, "vectors"))
	if err != nil {
		log.Fatal(err)
	}

	eng := engine.New(kvStore, chunkStore, vectorDB, graphStore)
	mode, err := engine.ParsePruningMode(*pruningMode)
	if err != nil {
		log.Fatal(err)
	}
	if err := eng.SetPruningMode(mode); err != nil {
		log.Fatal(err)
	}

	// ── Phase 1: Full index ─────────────────────────────────────────────
	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	fullStart := time.Now()
	if err := eng.IndexDir(context.Background(), absRepo, 0); err != nil {
		log.Fatal(err)
	}

	runtime.GC()
	runtime.GC()
	var memAfter runtime.MemStats
	runtime.ReadMemStats(&memAfter)

	if err := eng.FinalizeIndex(); err != nil {
		log.Fatal(err)
	}
	fullElapsed := time.Since(fullStart)

	var heapDeltaMB float64
	if memAfter.HeapInuse > memBefore.HeapInuse {
		heapDeltaMB = float64(memAfter.HeapInuse-memBefore.HeapInuse) / 1024 / 1024
	}

	stats := eng.Stats()
	nChunks := stats["chunks"]
	nGraphNodes := stats["graph_nodes"]
	nGraphEdges := stats["graph_edges"]

	// ── Phase 2: Incremental re-index (single file) ─────────────────────
	paths, _ := searcher.WalkFiles(absRepo, 0)
	var incrElapsed time.Duration
	if len(paths) > 0 {
		mid := paths[len(paths)/2]
		src, _ := os.ReadFile(mid)
		relPath, _ := filepath.Rel(absRepo, mid)
		t := time.Now()
		_ = eng.IndexFile(context.Background(), relPath, string(src))
		_ = eng.FinalizeIndex()
		incrElapsed = time.Since(t)
	}

	// ── Phase 3: Incremental sync (no changes — measures overhead) ─────
	// First sync populates the index store hashes so the second one is a true no-op.
	syncer := indexer.NewSyncer(eng, indexStore, absRepo)
	_, _, _ = syncer.Sync(context.Background())
	syncStart := time.Now()
	_, _, _ = syncer.Sync(context.Background())
	noopSyncElapsed := time.Since(syncStart)

	// ── Phase 4: Harvest real symbols from the indexed graph ────────────
	symbols := harvestSymbols(eng, chunkStore)

	ctx := context.Background()

	// ── Phase 5: Query benchmarks ───────────────────────────────────────
	searchResult := benchOp("search", symbols.searchQueries, 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QuerySearch, Text: q, TopK: 10})
	})
	searchFilteredResult := benchOp("search+filter", symbols.searchQueries[:min(5, len(symbols.searchQueries))], 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QuerySearch, Text: q, TopK: 10, Language: "go"})
	})
	defResult := benchOp("find_definition", symbols.defQueries, 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryDefinition, Text: q})
	})
	refResult := benchOp("find_references", symbols.refQueries, 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryReferences, Text: q})
	})
	callerResult := benchOp("get_callers", symbols.callerQueries, 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryCallers, Text: q})
	})
	calleeResult := benchOp("get_callees", symbols.calleeQueries, 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryCallees, Text: q})
	})
	impactResult := benchOp("impact_analysis", symbols.impactQueries, 20, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryImpact, Text: q, MaxDepth: 3})
	})
	contextResult := benchOp("get_context_pack", symbols.contextQueries, 20, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryContextPack, Text: q})
	})
	snippetResult := benchOp("read_snippet", symbols.snippetQueries, 30, func(q string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryReadSnippet, Text: q})
	})

	// ── Phase 6: Binary size ────────────────────────────────────────────
	var binarySizeMB float64
	if info, err := os.Stat(os.Args[0]); err == nil {
		binarySizeMB = float64(info.Size()) / 1024 / 1024
	}

	// ── Output ──────────────────────────────────────────────────────────
	fmt.Println()
	fmt.Println("smart-rag performance benchmark")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Printf("  Version      : %s\n", version)
	fmt.Printf("  Repository   : %s\n", absRepo)
	fmt.Printf("  Source files : %d  (%d lines)\n", totalFiles, totalLines)
	fmt.Printf("  Pruning      : %s\n", *pruningMode)
	fmt.Println()

	fmt.Println("  Index Stats")
	fmt.Println("  ───────────────────────────────────────────────────────────")
	fmt.Printf("  Chunks       : %d\n", nChunks)
	fmt.Printf("  Graph nodes  : %d\n", nGraphNodes)
	fmt.Printf("  Graph edges  : %d\n", nGraphEdges)
	fmt.Println()

	fmt.Println("  Indexing Performance")
	fmt.Println("  ───────────────────────────────────────────────────────────")
	fmt.Printf("  Full index         : %-12s  (%d files)\n", fullElapsed.Round(time.Millisecond), totalFiles)
	if totalFiles > 0 {
		fmt.Printf("  Per file (avg)     : %s\n", (fullElapsed / time.Duration(totalFiles)).Round(time.Microsecond))
	}
	fmt.Printf("  Incremental 1-file : %s\n", incrElapsed.Round(time.Millisecond))
	fmt.Printf("  No-op sync         : %s\n", noopSyncElapsed.Round(time.Millisecond))
	if heapDeltaMB > 0 {
		fmt.Printf("  Heap delta         : %.1f MB\n", heapDeltaMB)
	}
	if binarySizeMB > 0 {
		fmt.Printf("  Binary size        : %.1f MB\n", binarySizeMB)
	}
	fmt.Println()

	fmt.Println("  Query Latency")
	fmt.Println("  ───────────────────────────────────────────────────────────")
	fmt.Println("  Operation            Queries   Median     P95        P99        Min        Max")
	printBenchRow(searchResult)
	printBenchRow(searchFilteredResult)
	printBenchRow(defResult)
	printBenchRow(refResult)
	printBenchRow(callerResult)
	printBenchRow(calleeResult)
	printBenchRow(impactResult)
	printBenchRow(contextResult)
	printBenchRow(snippetResult)
	fmt.Println()
}

type symbolSet struct {
	searchQueries  []string
	defQueries     []string
	refQueries     []string
	callerQueries  []string
	calleeQueries  []string
	impactQueries  []string
	contextQueries []string
	snippetQueries []string
}

func harvestSymbols(eng *engine.Engine, cs *storage.ChunkStore) symbolSet {
	var s symbolSet

	chunks, err := cs.GetAll()
	if err != nil || len(chunks) == 0 {
		s.searchQueries = []string{"parse", "index", "search"}
		return s
	}

	funcNames := make(map[string]bool)
	typeNames := make(map[string]bool)
	var chunkIDs []string
	var snippetLocs []string

	for _, ch := range chunks {
		if ch.SymbolName != "" {
			if strings.HasPrefix(ch.Signature, "func ") || ch.ChunkType == "4" {
				funcNames[ch.SymbolName] = true
			} else {
				typeNames[ch.SymbolName] = true
			}
		}
		chunkIDs = append(chunkIDs, ch.ID)
		if ch.StartLine > 0 {
			snippetLocs = append(snippetLocs, fmt.Sprintf("%s:%d-%d", ch.FilePath, ch.StartLine, ch.EndLine))
		}
	}

	stats := eng.Stats()
	graphNodes := stats["graph_nodes"]

	funcs := sortedKeys(funcNames)
	types := sortedKeys(typeNames)

	s.searchQueries = pickDistributed(append(funcs, types...), 15)

	s.defQueries = pickDistributed(append(types, funcs...), 10)

	s.refQueries = pickDistributed(append(funcs, types...), 8)

	if graphNodes > 0 {
		s.callerQueries = pickDistributed(funcs, 8)
		s.calleeQueries = pickDistributed(funcs, 8)
		s.impactQueries = pickDistributed(funcs, 6)
	} else {
		s.callerQueries = pickDistributed(funcs, 3)
		s.calleeQueries = pickDistributed(funcs, 3)
		s.impactQueries = pickDistributed(funcs, 2)
	}

	s.contextQueries = pickDistributed(chunkIDs, 8)
	s.snippetQueries = pickDistributed(snippetLocs, 10)

	return s
}

type benchResult struct {
	name       string
	n          int
	median     time.Duration
	p95        time.Duration
	p99        time.Duration
	minLat     time.Duration
	maxLat     time.Duration
}

func benchOp(name string, queries []string, repsPerQuery int, fn func(string)) benchResult {
	if len(queries) == 0 {
		return benchResult{name: name}
	}

	// warmup
	fn(queries[0])

	var lats []time.Duration
	for _, q := range queries {
		for range repsPerQuery {
			t := time.Now()
			fn(q)
			lats = append(lats, time.Since(t))
		}
	}

	sort.Slice(lats, func(i, j int) bool { return lats[i] < lats[j] })
	n := len(lats)
	return benchResult{
		name:   name,
		n:      n,
		median: lats[n/2],
		p95:    lats[percentileIdx(n, 0.95)],
		p99:    lats[percentileIdx(n, 0.99)],
		minLat: lats[0],
		maxLat: lats[n-1],
	}
}

func printBenchRow(r benchResult) {
	if r.n == 0 {
		fmt.Printf("  %-22s  %5s   %-10s %-10s %-10s %-10s %s\n",
			r.name, "-", "-", "-", "-", "-", "-")
		return
	}
	fmt.Printf("  %-22s  %5d   %-10s %-10s %-10s %-10s %s\n",
		r.name, r.n,
		fmtDur(r.median), fmtDur(r.p95), fmtDur(r.p99),
		fmtDur(r.minLat), fmtDur(r.maxLat))
}

func percentileIdx(n int, p float64) int {
	idx := int(math.Ceil(float64(n)*p)) - 1
	if idx < 0 {
		return 0
	}
	if idx >= n {
		return n - 1
	}
	return idx
}

func fmtDur(d time.Duration) string {
	if d < time.Microsecond {
		return "< 1µs"
	}
	if d < time.Millisecond {
		return fmt.Sprintf("%dµs", d.Microseconds())
	}
	return d.Round(time.Millisecond).String()
}

func repoStats(dir string) (files, lines int) {
	paths, _ := searcher.WalkFiles(dir, 0)
	for _, p := range paths {
		files++
		src, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		lines += bytes.Count(src, []byte{'\n'})
	}
	return
}

func sortedKeys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func pickDistributed(items []string, n int) []string {
	if len(items) == 0 {
		return nil
	}
	if len(items) <= n {
		return items
	}
	step := float64(len(items)) / float64(n)
	out := make([]string, 0, n)
	for i := range n {
		idx := int(float64(i) * step)
		if idx >= len(items) {
			idx = len(items) - 1
		}
		out = append(out, items[idx])
	}
	return out
}
