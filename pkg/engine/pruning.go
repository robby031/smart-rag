package engine

import (
	"sort"
	"strconv"
	"strings"

	"github.com/robby031/smart-rag/pkg/graph"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/storage"
)

const (
	ReachabilityUnknown     = "unknown"
	ReachabilityReachable   = "reachable"
	ReachabilityUnreachable = "unreachable"

	reachableContextWeight   = 1.0
	unreachableContextWeight = 0.55
	autoContextMinWeight     = 0.75
)

func (e *Engine) refreshChunkReachability() error {
	if e.callGraph == nil || e.chunkStore == nil {
		return nil
	}

	nodesByFileLine := make(map[string]string, len(e.callGraph.Nodes))
	for id, node := range e.callGraph.Nodes {
		nodesByFileLine[fileLineKey(node.File, node.Line)] = id
	}

	chunks, err := e.chunkStore.GetAll()
	if err != nil {
		return err
	}

	entrypointRoots := make([]string, 0)
	for _, chunk := range chunks {
		if !isEntrypointChunk(chunk) {
			continue
		}
		if nodeID, ok := nodesByFileLine[fileLineKey(chunk.FilePath, chunk.StartLine)]; ok {
			entrypointRoots = append(entrypointRoots, nodeID)
		}
	}

	reachable := e.reachableNodeSet(entrypointRoots...)
	updated := make([]storage.ChunkMeta, 0, len(chunks))
	for _, chunk := range chunks {
		meta := *chunk
		meta.Reachability = ReachabilityUnknown
		meta.ContextWeight = reachableContextWeight

		if isExportedTypeChunk(&meta) {
			meta.Reachability = ReachabilityReachable
			meta.ContextWeight = reachableContextWeight
		} else if isPrunableFunctionChunk(&meta) {
			if nodeID, ok := nodesByFileLine[fileLineKey(meta.FilePath, meta.StartLine)]; ok {
				if reachable[nodeID] {
					meta.Reachability = ReachabilityReachable
					meta.ContextWeight = reachableContextWeight
				} else {
					meta.Reachability = ReachabilityUnreachable
					meta.ContextWeight = unreachableContextWeight
				}
			}
		}

		updated = append(updated, meta)
	}

	return e.chunkStore.PutAll(updated)
}

func (e *Engine) reachableNodeSet(extraRoots ...string) map[string]bool {
	reachable := make(map[string]bool)
	if e.callGraph == nil {
		return reachable
	}

	roots := e.reachabilityRoots()
	roots = append(roots, extraRoots...)
	queue := append([]string(nil), roots...)
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if reachable[id] {
			continue
		}
		reachable[id] = true

		caller := e.callGraph.Nodes[id]
		for callee := range e.callGraph.OutEdges[id] {
			for _, resolved := range e.resolveCalleeIDs(caller, callee) {
				if !reachable[resolved] {
					queue = append(queue, resolved)
				}
			}
		}
	}

	return reachable
}

func (e *Engine) reachabilityRoots() []string {
	if e.callGraph == nil {
		return nil
	}

	roots := make([]string, 0)
	for id, node := range e.callGraph.Nodes {
		if isReachabilityRoot(node) {
			roots = append(roots, id)
		}
	}
	sort.Strings(roots)
	return roots
}

func isReachabilityRoot(node *graph.Node) bool {
	if node == nil {
		return false
	}
	if node.Name == "init" {
		return true
	}
	if node.Pkg == "main" && node.Name == "main" {
		return true
	}
	if strings.HasSuffix(node.File, "_test.go") &&
		(strings.HasPrefix(node.Name, "Test") ||
			strings.HasPrefix(node.Name, "Benchmark") ||
			strings.HasPrefix(node.Name, "Fuzz") ||
			strings.HasPrefix(node.Name, "Example")) {
		return true
	}
	return indexer.IsExported(node.Name)
}

func isEntrypointChunk(chunk *storage.ChunkMeta) bool {
	if chunk == nil || !isPrunableFunctionChunk(chunk) {
		return false
	}
	if isHTTPHandlerChunk(chunk) {
		return true
	}
	return isCLICommandChunk(chunk)
}

func isHTTPHandlerChunk(chunk *storage.ChunkMeta) bool {
	name := strings.ToLower(chunk.SymbolName)
	signature := strings.ToLower(chunk.Signature)
	content := strings.ToLower(chunk.Content)

	if chunk.SymbolName == "ServeHTTP" {
		return true
	}
	if strings.Contains(signature, "http.responsewriter") && strings.Contains(signature, "*http.request") {
		return true
	}
	if strings.HasPrefix(name, "handle") || strings.HasSuffix(name, "handler") {
		return true
	}
	return strings.Contains(content, "http.handlefunc(") || strings.Contains(content, ".handlefunc(")
}

func isCLICommandChunk(chunk *storage.ChunkMeta) bool {
	name := strings.ToLower(chunk.SymbolName)
	content := strings.ToLower(chunk.Content)

	switch name {
	case "execute", "run", "rune", "prerun", "postrun", "persistentprerun", "persistentpostrun":
		return true
	}
	if strings.Contains(chunk.FilePath, "/cmd/") || strings.HasPrefix(chunk.FilePath, "cmd/") {
		return true
	}
	if strings.HasPrefix(name, "run") ||
		strings.HasSuffix(name, "command") ||
		strings.HasPrefix(name, "newcommand") ||
		strings.HasPrefix(name, "new") && strings.HasSuffix(name, "cmd") {
		return true
	}
	return strings.Contains(content, "cobra.command") ||
		strings.Contains(content, "cli.command") ||
		strings.Contains(content, "urfave/cli") ||
		strings.Contains(content, "run:") ||
		strings.Contains(content, "rune:") ||
		strings.Contains(content, "action:")
}

func (e *Engine) resolveCalleeIDs(caller *graph.Node, callee string) []string {
	if e.callGraph == nil || callee == "" || callee == "<anonymous>" {
		return nil
	}
	if _, ok := e.callGraph.Nodes[callee]; ok {
		return []string{callee}
	}

	candidates := make(map[string]bool)
	if caller != nil {
		localID := caller.Pkg + "." + callee
		if _, ok := e.callGraph.Nodes[localID]; ok {
			candidates[localID] = true
		}
	}

	name := lastSelector(callee)
	if name != "" && caller != nil {
		for id, node := range e.callGraph.Nodes {
			if node.Pkg == caller.Pkg && node.Name == name {
				candidates[id] = true
			}
		}
	}

	if len(candidates) == 0 {
		return nil
	}
	out := make([]string, 0, len(candidates))
	for id := range candidates {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func isPrunableFunctionChunk(chunk *storage.ChunkMeta) bool {
	if chunk == nil || chunk.SymbolName == "" {
		return false
	}
	return chunk.ChunkType == "4" || strings.HasPrefix(chunk.Signature, "func ")
}

func isExportedTypeChunk(chunk *storage.ChunkMeta) bool {
	if chunk == nil || chunk.SymbolName == "" || !indexer.IsExported(chunk.SymbolName) {
		return false
	}
	switch chunk.ChunkType {
	case "3", "5", "6":
		return true
	default:
		return false
	}
}

func fileLineKey(filePath string, line int) string {
	return filePath + ":" + strconv.Itoa(line)
}

func lastSelector(symbol string) string {
	if idx := strings.LastIndex(symbol, "."); idx >= 0 && idx+1 < len(symbol) {
		return symbol[idx+1:]
	}
	return symbol
}

func chunkContextWeight(chunk *storage.ChunkMeta) float64 {
	if chunk == nil || chunk.ContextWeight <= 0 {
		return reachableContextWeight
	}
	return chunk.ContextWeight
}

func chunkAutoContextEligible(chunk *storage.ChunkMeta) bool {
	if chunk == nil {
		return false
	}
	if chunk.Reachability == ReachabilityUnreachable {
		return false
	}
	return chunkContextWeight(chunk) >= autoContextMinWeight
}

func (e *Engine) queryReachableChunkSet(query string, queryTokens map[string]int) map[string]bool {
	if e.callGraph == nil || e.chunkStore == nil {
		return nil
	}

	var roots []string
	normalizedQuery := normalizeSearchText(query)
	for id, node := range e.callGraph.Nodes {
		if nodeMatchesQuery(node, normalizedQuery, queryTokens) {
			roots = append(roots, id)
		}
	}
	if len(roots) == 0 {
		return nil
	}

	reachableNodes := e.reachableNodeSet(roots...)
	chunks, err := e.chunkStore.GetAll()
	if err != nil {
		return nil
	}
	nodesByFileLine := make(map[string]bool, len(reachableNodes))
	for id := range reachableNodes {
		node := e.callGraph.Nodes[id]
		if node == nil {
			continue
		}
		nodesByFileLine[fileLineKey(node.File, node.Line)] = true
	}

	out := make(map[string]bool)
	for _, chunk := range chunks {
		if nodesByFileLine[fileLineKey(chunk.FilePath, chunk.StartLine)] {
			out[chunk.ID] = true
		}
	}
	return out
}

func nodeMatchesQuery(node *graph.Node, normalizedQuery string, queryTokens map[string]int) bool {
	if node == nil {
		return false
	}
	if normalizedQuery != "" {
		if normalizeSearchText(node.Name) == normalizedQuery || normalizeSearchText(node.ID()) == normalizedQuery {
			return true
		}
	}

	name := strings.ToLower(node.Name)
	id := strings.ToLower(node.ID())
	matched := 0
	for term := range queryTokens {
		if strings.Contains(name, term) || strings.Contains(id, term) {
			matched++
		}
	}
	return matched > 0 && matched == len(queryTokens)
}
