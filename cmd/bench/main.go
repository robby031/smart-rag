package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/robby031/smart-rag/pkg/engine"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/searcher"
	"github.com/robby031/smart-rag/pkg/storage"
)

var version = "dev"

func main() {
	repoDir := flag.String("repo", ".", "Path to the code repository")
	dbDir := flag.String("db", "./rag-data", "Path to the RAG database")
	fullReindex := flag.Bool("full", false, "Force full re-index")
	flag.Parse()

	absRepo, _ := filepath.Abs(*repoDir)
	absDB, _ := filepath.Abs(*dbDir)
	os.MkdirAll(absDB, 0755)

	goFiles, totalLines := goStats(absRepo)

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

	// Full or incremental index
	var (
		indexedCount int
		peakHeapMB   float64
	)

	runtime.GC()
	var memBefore runtime.MemStats
	runtime.ReadMemStats(&memBefore)

	start := time.Now()

	if *fullReindex {
		fmt.Println("Full re-indexing:", absRepo)
		if err := eng.IndexDir(context.Background(), absRepo, 0); err != nil {
			log.Fatal(err)
		}
		// Two GC passes: first collects the main garbage, second collects
		// finalizer-triggered objects and inter-generational references.
		runtime.GC()
		runtime.GC()
		var memPeak runtime.MemStats
		runtime.ReadMemStats(&memPeak)
		if memPeak.HeapInuse > memBefore.HeapInuse {
			peakHeapMB = float64(memPeak.HeapInuse-memBefore.HeapInuse) / 1024 / 1024
		}
		if err := eng.FinalizeIndex(); err != nil {
			log.Fatal(err)
		}
		indexedCount = goFiles
	} else {
		syncer := indexer.NewSyncer(eng, indexStore, absRepo)
		indexed, deleted, err := syncer.Sync(context.Background())
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("Incremental: %d indexed, %d removed\n", indexed, deleted)
		indexedCount = indexed
	}

	elapsed := time.Since(start)
	s := eng.Stats()

	var projected time.Duration
	if indexedCount > 0 {
		projected = time.Duration(float64(elapsed) * 1000.0 / float64(indexedCount))
	}

	// Incremental re-index (1 file)
	var incrElapsed time.Duration
	if *fullReindex {
		paths, _ := searcher.WalkFiles(absRepo, 0)
		if len(paths) > 0 {
			src, _ := os.ReadFile(paths[0])
			relPath, _ := filepath.Rel(absRepo, paths[0])
			t := time.Now()
			_ = eng.IndexFile(context.Background(), relPath, string(src))
			_ = eng.FinalizeIndex()
			incrElapsed = time.Since(t)
		}
	}

	// Query latency (search + definition + callers)
	ctx := context.Background()
	nChunks := s["chunks"]

	searchLat := benchQueries(func(term string) {
		eng.Query(ctx, engine.Query{Type: engine.QuerySearch, Text: term, TopK: 10}) //nolint
	}, []string{"Parse", "Index", "node", "chunk", "graph", "search", "token", "file", "error", "engine"}, 20)

	defLat := benchQueries(func(term string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryDefinition, Text: term}) //nolint
	}, []string{"ParseFile", "IndexFile", "AddNode", "AddEdge", "PutAll", "Flush", "BatchPut", "WalkFiles"}, 20)

	callerLat := benchQueries(func(term string) {
		eng.Query(ctx, engine.Query{Type: engine.QueryCallers, Text: term}) //nolint
	}, []string{"ParseFile", "IndexFile", "AddNode", "Flush"}, 20)

	// Binary size
	var binarySizeMB float64
	if info, err := os.Stat(os.Args[0]); err == nil {
		binarySizeMB = float64(info.Size()) / 1024 / 1024
	}

	// Output
	fmt.Println()
	fmt.Println("smart-rag performance matrix")
	fmt.Println("================================")
	fmt.Printf("  Version       : %s\n", version)
	fmt.Printf("  Repository    : %s\n", absRepo)
	fmt.Printf("  Go files      : %d (%d lines)\n", goFiles, totalLines)
	fmt.Printf("  Chunks        : %d\n", nChunks)
	fmt.Printf("  Graph nodes   : %d\n", s["graph_nodes"])
	fmt.Printf("  Graph edges   : %d\n", s["graph_edges"])
	fmt.Printf("  Index time    : %s\n", elapsed.Round(time.Millisecond))
	fmt.Println("--------------------------------")
	fmt.Println("  Metric                      Target        Actual")

	// Cold index
	status := statusIcon(elapsed, 5*time.Second, 8*time.Second)
	fmt.Printf("  Cold index (%4d files)  %s  < 5-8s       %s\n", goFiles, status, elapsed.Round(time.Millisecond))

	// Projected 1k
	projStatus := statusIcon(projected, 5*time.Second, 8*time.Second)
	if indexedCount > 0 {
		fmt.Printf("  Projected   (1000 files) %s  < 5-8s       ~%s  [from %d files]\n", projStatus, projected.Round(time.Millisecond), indexedCount)
	} else {
		fmt.Printf("  Projected   (1000 files)     < 5-8s       n/a\n")
	}

	// Incremental
	if incrElapsed > 0 {
		incrStatus := statusIcon(incrElapsed, 1*time.Second, 2*time.Second)
		fmt.Printf("  Incremental (    1 file) %s  < 1-2s       %s\n", incrStatus, incrElapsed.Round(time.Millisecond))
	} else {
		fmt.Printf("  Incremental (    1 file)     < 1-2s       (run --full to measure)\n")
	}

	// Query latency – search
	smed, sp95 := medianAndP95(searchLat)
	sStatus := statusIcon(smed, 50*time.Millisecond, 80*time.Millisecond)
	fmt.Printf("  Query search             %s  < 50-80ms    median %-8s  p95 %s  [%d chunks]\n",
		sStatus, fmtDur(smed), fmtDur(sp95), nChunks)

	// Query latency – definition
	dmed, dp95 := medianAndP95(defLat)
	dStatus := statusIcon(dmed, 50*time.Millisecond, 80*time.Millisecond)
	fmt.Printf("  Query find-def           %s  < 50-80ms    median %-8s  p95 %s  [%d chunks]\n",
		dStatus, fmtDur(dmed), fmtDur(dp95), nChunks)

	// Query latency – callers
	cmed, cp95 := medianAndP95(callerLat)
	cStatus := statusIcon(cmed, 50*time.Millisecond, 80*time.Millisecond)
	fmt.Printf("  Query callers            %s  < 50-80ms    median %-8s  p95 %s  [%d chunks]\n",
		cStatus, fmtDur(cmed), fmtDur(cp95), nChunks)

	// Binary size
	if binarySizeMB > 0 {
		bStatus := statusIcon(time.Duration(binarySizeMB*1000)*time.Millisecond, 15000*time.Millisecond, 20000*time.Millisecond)
		fmt.Printf("  Binary size              %s  < 15-20 MB   %.1f MB\n", bStatus, binarySizeMB)
	}

	// RAM
	if peakHeapMB > 0 {
		ramStatus := statusIcon(time.Duration(peakHeapMB*1000)*time.Millisecond, 80000*time.Millisecond, 120000*time.Millisecond)
		fmt.Printf("  RAM during index         %s  < 80-120 MB  %.1f MB heap delta\n", ramStatus, peakHeapMB)
	} else {
		fmt.Printf("  RAM during index             < 80-120 MB  (run --full to measure)\n")
	}

	if nChunks > 0 && smed > 0 {
		proj100k := time.Duration(float64(smed) * 100_000 / float64(nChunks))
		p100kStatus := statusIcon(proj100k, 40*time.Millisecond, 100*time.Millisecond)
		fmt.Printf("  Query 100k docs          %s  ~20-40ms     ~%s projected  [linear from %d chunks]\n",
			p100kStatus, fmtDur(proj100k), nChunks)
	}

	fmt.Println()
}

func benchQueries(fn func(string), terms []string, repsPerTerm int) []time.Duration {
	out := make([]time.Duration, 0, len(terms)*repsPerTerm)
	for _, term := range terms {
		for range repsPerTerm {
			t := time.Now()
			fn(term)
			out = append(out, time.Since(t))
		}
	}
	return out
}

func medianAndP95(d []time.Duration) (median, p95 time.Duration) {
	if len(d) == 0 {
		return 0, 0
	}
	cp := make([]time.Duration, len(d))
	copy(cp, d)
	sort.Slice(cp, func(i, j int) bool { return cp[i] < cp[j] })
	median = cp[len(cp)/2]
	p95 = cp[int(float64(len(cp))*0.95)]
	return
}

func statusIcon(actual, warn, crit time.Duration) string {
	switch {
	case actual <= warn:
		return "ok"
	case actual <= crit:
		return "warn"
	default:
		return "crit"
	}
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

func goStats(dir string) (files, lines int) {
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
