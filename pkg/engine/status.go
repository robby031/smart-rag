package engine

import (
	"strconv"
	"time"
)

type RuntimeInfo struct {
	Version string
	RepoDir string
	DBDir   string
}

type IndexSummary struct {
	Mode      string
	Indexed   int
	Deleted   int
	UpdatedAt time.Time
}

type Status struct {
	Version          string
	RepoDir          string
	DBDir            string
	IndexedChunks    int
	GraphNodes       int
	GraphEdges       int
	BM25Ready        bool
	BM25Empty        bool
	LastIndexSummary string
}

func (e *Engine) SetRuntimeInfo(info RuntimeInfo) {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	e.runtimeInfo = info
}

func (e *Engine) RecordIndexSummary(summary IndexSummary) {
	e.statusMu.Lock()
	defer e.statusMu.Unlock()
	if summary.UpdatedAt.IsZero() {
		summary.UpdatedAt = time.Now()
	}
	e.lastIndexSummary = summary
}

func (e *Engine) Status() Status {
	e.indexMu.RLock()
	graphStats := map[string]int{"nodes": 0, "edges": 0}
	if e.callGraph != nil {
		graphStats = e.callGraph.Stats()
	}
	bm25Empty := true
	indexedChunks := 0
	if e.bm25 != nil {
		bm25Empty = e.bm25.IsEmpty()
		indexedChunks = len(e.bm25.DocIDs)
	}
	e.indexMu.RUnlock()

	e.statusMu.RLock()
	runtimeInfo := e.runtimeInfo
	lastIndexSummary := formatIndexSummary(e.lastIndexSummary)
	e.statusMu.RUnlock()

	return Status{
		Version:          runtimeInfo.Version,
		RepoDir:          runtimeInfo.RepoDir,
		DBDir:            runtimeInfo.DBDir,
		IndexedChunks:    indexedChunks,
		GraphNodes:       graphStats["nodes"],
		GraphEdges:       graphStats["edges"],
		BM25Ready:        !bm25Empty,
		BM25Empty:        bm25Empty,
		LastIndexSummary: lastIndexSummary,
	}
}

func formatIndexSummary(summary IndexSummary) string {
	if summary.Mode == "" && summary.Indexed == 0 && summary.Deleted == 0 && summary.UpdatedAt.IsZero() {
		return "unavailable"
	}
	mode := summary.Mode
	if mode == "" {
		mode = "unknown"
	}
	return mode + ": indexed=" + strconv.Itoa(summary.Indexed) + " deleted=" + strconv.Itoa(summary.Deleted) + " updated_at=" + summary.UpdatedAt.Format(time.RFC3339)
}
