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

	SemanticRoleBoilerplate = "boilerplate"

	FoldReasonGeneratedCode      = "generated_code"
	FoldReasonTrivialConstructor = "trivial_constructor"
	FoldReasonGetterSetter       = "getter_setter"
	FoldReasonLargeDTO           = "large_dto"
	FoldReasonErrorConstBlock    = "error_const_block"
	FoldReasonSimpleWrapper      = "simple_wrapper"
	FoldReasonTrivialDeclaration = "trivial_declaration"

	reachableContextWeight   = 1.0
	unreachableContextWeight = 0.55
	boilerplateContextWeight = 0.65
	generatedContextWeight   = 0.40
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
		meta.ContextWeight = baseContextWeight(&meta)

		if isExportedTypeChunk(&meta) {
			meta.Reachability = ReachabilityReachable
			meta.ContextWeight = baseContextWeight(&meta)
		} else if isPrunableFunctionChunk(&meta) {
			if nodeID, ok := nodesByFileLine[fileLineKey(meta.FilePath, meta.StartLine)]; ok {
				if reachable[nodeID] {
					meta.Reachability = ReachabilityReachable
					meta.ContextWeight = baseContextWeight(&meta)
				} else {
					meta.Reachability = ReachabilityUnreachable
					meta.ContextWeight = unreachableContextWeight
				}
			}
		}
		applySemanticFolding(&meta)

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

func applySemanticFolding(chunk *storage.ChunkMeta) {
	if chunk == nil {
		return
	}

	reason := chunk.FoldReason
	if reason == "" {
		reason = detectFoldReason(chunk)
	}
	if reason == "" {
		chunk.SemanticRole = ""
		chunk.FoldReason = ""
		return
	}

	chunk.SemanticRole = SemanticRoleBoilerplate
	chunk.FoldReason = reason
	weight := boilerplateContextWeight
	if reason == FoldReasonGeneratedCode {
		weight = generatedContextWeight
	}
	if chunk.ContextWeight <= 0 || weight < chunk.ContextWeight {
		chunk.ContextWeight = weight
	}
}

func detectFoldReason(chunk *storage.ChunkMeta) string {
	content := strings.TrimSpace(chunk.Content)
	if content == "" {
		return ""
	}
	lowerContent := strings.ToLower(content)

	if isGeneratedChunk(content) {
		return FoldReasonGeneratedCode
	}
	if isErrorConstBlock(chunk, content, lowerContent) {
		return FoldReasonErrorConstBlock
	}
	if isLargeDTOChunk(chunk, content) {
		return FoldReasonLargeDTO
	}
	if isTrivialDeclaration(chunk, content) {
		return FoldReasonTrivialDeclaration
	}
	if !isPrunableFunctionChunk(chunk) {
		return ""
	}
	if isGetterSetter(chunk, content) {
		return FoldReasonGetterSetter
	}
	if isTrivialConstructor(chunk, content) {
		return FoldReasonTrivialConstructor
	}
	if isSimpleWrapper(chunk, content) {
		return FoldReasonSimpleWrapper
	}
	return ""
}

func baseContextWeight(chunk *storage.ChunkMeta) float64 {
	if chunk == nil || chunk.ContextWeight <= 0 {
		return reachableContextWeight
	}
	return chunk.ContextWeight
}

func isGeneratedSource(src string) bool {
	for i, line := range strings.Split(src, "\n") {
		if i >= 20 {
			break
		}
		line = strings.ToLower(line)
		if strings.Contains(line, "code generated") && strings.Contains(line, "do not edit") {
			return true
		}
	}
	return false
}

func isGeneratedChunk(content string) bool {
	lower := strings.ToLower(content)
	return strings.Contains(lower, "code generated") && strings.Contains(lower, "do not edit")
}

func isErrorConstBlock(chunk *storage.ChunkMeta, content, lowerContent string) bool {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(chunk.SymbolName, "Err") && strings.Contains(trimmed, "=") {
		return true
	}
	if !strings.HasPrefix(trimmed, "const ") && !strings.HasPrefix(trimmed, "var ") {
		return false
	}
	if strings.Contains(lowerContent, "errors.new(") || strings.Contains(lowerContent, "fmt.errorf(") {
		return true
	}
	return strings.Contains(content, "Err") || strings.Contains(lowerContent, "error")
}

func isLargeDTOChunk(chunk *storage.ChunkMeta, content string) bool {
	if chunk == nil || chunk.ChunkType != "5" {
		return false
	}
	fieldLines := 0
	tagLines := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") || line == "{" || line == "}" {
			continue
		}
		if strings.HasPrefix(line, "type ") || strings.HasPrefix(line, "struct") {
			continue
		}
		fieldLines++
		if strings.Contains(line, "`json:") || strings.Contains(line, "`db:") || strings.Contains(line, "`yaml:") {
			tagLines++
		}
	}
	name := strings.ToLower(chunk.SymbolName)
	nameLooksDTO := strings.HasSuffix(name, "dto") ||
		strings.HasSuffix(name, "request") ||
		strings.HasSuffix(name, "response") ||
		strings.HasSuffix(name, "payload") ||
		strings.HasSuffix(name, "model")
	return fieldLines >= 8 && (tagLines >= fieldLines/2 || nameLooksDTO)
}

func isTrivialDeclaration(chunk *storage.ChunkMeta, content string) bool {
	if chunk == nil {
		return false
	}
	switch chunk.ChunkType {
	case "3", "5", "6":
		return countMeaningfulLines(content) <= 3
	default:
		return false
	}
}

func isGetterSetter(chunk *storage.ChunkMeta, content string) bool {
	name := chunk.SymbolName
	if !(strings.HasPrefix(name, "Get") || strings.HasPrefix(name, "Set") || strings.HasPrefix(name, "Is") || strings.HasPrefix(name, "Has")) {
		return false
	}
	body := functionBody(content)
	if body == "" {
		return false
	}
	statements := splitStatements(body)
	if len(statements) != 1 {
		return false
	}
	stmt := strings.TrimSpace(statements[0])
	if strings.HasPrefix(stmt, "return ") {
		expr := strings.TrimSpace(strings.TrimPrefix(stmt, "return "))
		return isFieldAccess(expr)
	}
	if strings.Contains(stmt, "=") && !strings.Contains(stmt, ":=") {
		parts := strings.SplitN(stmt, "=", 2)
		return isFieldAccess(strings.TrimSpace(parts[0]))
	}
	return false
}

func isTrivialConstructor(chunk *storage.ChunkMeta, content string) bool {
	name := chunk.SymbolName
	if !(strings.HasPrefix(name, "New") || strings.HasPrefix(name, "MustNew")) {
		return false
	}
	body := functionBody(content)
	if body == "" {
		return false
	}
	statements := splitStatements(body)
	if len(statements) != 1 {
		return false
	}
	stmt := strings.TrimSpace(statements[0])
	if !strings.HasPrefix(stmt, "return ") {
		return false
	}
	expr := strings.TrimSpace(strings.TrimPrefix(stmt, "return "))
	if strings.HasPrefix(expr, "&") {
		expr = strings.TrimSpace(strings.TrimPrefix(expr, "&"))
	}
	return strings.Contains(expr, "{") && strings.HasSuffix(expr, "}")
}

func isSimpleWrapper(chunk *storage.ChunkMeta, content string) bool {
	if isEntrypointChunk(chunk) {
		return false
	}
	body := functionBody(content)
	if body == "" || countMeaningfulLines(body) > 3 {
		return false
	}
	statements := splitStatements(body)
	if len(statements) != 1 {
		return false
	}
	stmt := strings.TrimSpace(statements[0])
	if strings.HasPrefix(stmt, "return ") {
		stmt = strings.TrimSpace(strings.TrimPrefix(stmt, "return "))
	}
	return looksLikeSingleCall(stmt)
}

func functionBody(content string) string {
	start := strings.Index(content, "{")
	end := strings.LastIndex(content, "}")
	if start < 0 || end <= start {
		return ""
	}
	return strings.TrimSpace(content[start+1 : end])
}

func splitStatements(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		for _, part := range strings.Split(line, ";") {
			part = strings.TrimSpace(part)
			if part != "" {
				out = append(out, part)
			}
		}
	}
	return out
}

func countMeaningfulLines(content string) int {
	count := 0
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		count++
	}
	return count
}

func isFieldAccess(expr string) bool {
	expr = strings.TrimSpace(expr)
	return strings.Contains(expr, ".") && !strings.Contains(expr, "(") && !strings.Contains(expr, "[")
}

func looksLikeSingleCall(stmt string) bool {
	stmt = strings.TrimSpace(stmt)
	if strings.Contains(stmt, "{") || strings.Contains(stmt, "}") {
		return false
	}
	open := strings.Index(stmt, "(")
	close := strings.LastIndex(stmt, ")")
	if open <= 0 || close != len(stmt)-1 {
		return false
	}
	prefix := strings.TrimSpace(stmt[:open])
	if prefix == "" {
		return false
	}
	if strings.ContainsAny(prefix, "+-*/%<>=!&|") {
		return false
	}
	return true
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
