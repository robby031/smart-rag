# Smart RAG Setup Guide

**Purpose:** Guide for AI assistants to help users install smart-rag independently.

---

## Overview

Smart RAG is a code intelligence engine based on RAG (Retrieval-Augmented Generation) that indexes your codebase and provides MCP tools for Claude Desktop / Claude Code. With smart-rag, you can:
- Search code with natural language using ranked BM25
- Go-to-definition for symbols (find_definition)
- Find all references of a symbol (find_references)
- Explore call graph (get_callers / get_callees)
- Impact analysis (blast radius of changes)
- Read code snippets by file:line

**Supported languages:** Go, JavaScript, TypeScript (`.js`, `.jsx`, `.ts`, `.tsx`, `.mjs`, `.cjs`)

Smart RAG runs as an MCP server that communicates via JSON-RPC on stdio.

---

## Prerequisites

Choose one setup method based on your preference:

### Method 1: Docker via GHCR (Recommended - No build required)
- Docker installed (`docker --version`)
- Docker can run containers with volume mounts
- Path to your repository (local or remote)

### Method 2: Docker Build Locally
- Docker installed
- Git installed
- Path to your repository

### Method 3: Binary Without Docker
- Go 1.23+ installed (`go version`)
- Git installed
- OS: macOS/Linux/Windows

---

## Setup Step-by-Step

### Method 1: Docker via GHCR

**Step 1: Pull image**
```bash
docker pull ghcr.io/robby031/smart-rag:latest
```

**Step 2: Index your repository (first time)**
```bash
# Replace /path/to/your/project with actual path
docker run --rm \
  -v "/path/to/your/project:/repo:ro" \
  -v "smart-rag-data:/data" \
  ghcr.io/robby031/smart-rag:latest --repo=/repo --db=/data --full
```

Output will display:
```
Full re-indexing repository: /repo
Chunks: XXX
Graph nodes: XXX
Graph edges: XXX
Incremental indexing: XXX files indexed
Starting smart-rag MCP server...
```

If error "bind: address already in use", stop lingering container:
```bash
docker stop smart-rag
docker rm smart-rag
```

**Step 3: Add to Claude Desktop**

Open or create file:
```
~/Library/Application Support/Claude/claude_desktop_config.json
```

Add/update `mcpServers` section (replace path):
```json
{
  "mcpServers": {
    "smart-rag": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/your/project:/repo:ro",
        "-v", "smart-rag-data:/data",
        "ghcr.io/robby031/smart-rag:latest"
      ]
    }
  }
}
```

Restart Claude Desktop.

**Step 4: Add to Claude Code**

Create/update `.mcp.json` in project root (replace path):
```json
{
  "mcpServers": {
    "smart-rag": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/your/project:/repo:ro",
        "-v", "smart-rag-data:/data",
        "ghcr.io/robby031/smart-rag:latest"
      ]
    }
  }
}
```

Restart Claude Code / refresh MCP connection.

**Update image to latest:**
```bash
docker pull ghcr.io/robby031/smart-rag:latest
```

---

### Method 2: Docker Build Locally

**Step 1: Clone repository**
```bash
git clone https://github.com/robby031/smart-rag.git
cd smart-rag
```

**Step 2: Build image**
```bash
make docker-build
# Output: smart-rag:latest (13.2 MB)
```

**Step 3: Index your repository**
```bash
make docker-index REPO=/path/to/your/project
```

**Step 4: Configure MCP** (same as Method 1)

Update config file, replace image URL from:
```
ghcr.io/robby031/smart-rag:latest
```
to:
```
smart-rag:latest
```

**Step 5: Rebuild after updating**
```bash
make docker-restart REPO=/path/to/your/project
```

---

### Method 3: Binary Without Docker

**Step 1: Clone repository**
```bash
git clone https://github.com/robby031/smart-rag.git
cd smart-rag
```

**Step 2: Build**
```bash
make build
# Output: ./rag-mcp (binary)
```

**Step 3: Index your repository**
```bash
./rag-mcp --repo=/path/to/your/project --db=./rag-data --full
```

**Step 4: Install to GOBIN**
```bash
make install
# Binary available at $GOBIN/rag-mcp
```

**Step 5: Configure MCP**

Add to Claude Desktop / Claude Code config:
```json
{
  "mcpServers": {
    "smart-rag": {
      "command": "rag-mcp",
      "args": ["--repo", "/path/to/your/project"]
    }
  }
}
```

---

## Verification

After setup, test in Claude Desktop:

1. Open Chat
2. Click tools icon at bottom → Smart RAG tools will be visible
3. Try queries:
   - `rag_status`: "check smart-rag health"
   - `search_code`: "keyword search in codebase"
   - `find_definition`: symbol name — works for Go and JS/TS (functions, classes, types, enums)
   - `get_callers` / `get_callees`: function ID format:
     - Go: `pkg.FuncName` or `pkg.(ReceiverType).Method`
     - JS/TS: `module.funcName` or `module.(ClassName).method`
   - `impact_analysis`: "function or package/module name"

If tools don't appear, check:
- Config file has valid JSON (use jsonlint)
- Path `/path/to/your/project` is correct and readable
- Docker image available: `docker images | grep smart-rag`
- Container logs: `docker logs smart-rag`

---

## Troubleshooting

### Error: "bind: address already in use"
**Solution:** Old container still running.
```bash
docker ps | grep smart-rag
docker stop <container-id>
```

### Error: "Unexpected token 'I', 'Incrementa'... is not valid JSON"
**Cause:** stdout contaminated with non-JSON output.
**Solution:** Ensure latest version (`docker pull ghcr.io/robby031/smart-rag:latest`).

### Error: "chunk not found" / "file not found"
**Cause:** Re-indexing incomplete or path incorrect.
**Solution:** Run indexing again:
```bash
# Method 1 (GHCR)
docker run --rm -v "/path:/repo:ro" -v "smart-rag-data:/data" \
  ghcr.io/robby031/smart-rag:latest --repo=/repo --db=/data --full

# Method 2 (Local)
make docker-restart REPO=/path/to/project

# Method 3 (Binary)
./rag-mcp --repo=/path/to/project --db=./rag-data --full
```

### MCP tools not appearing in Claude
**Solution:**
1. Verify config JSON is valid
2. Restart Claude completely (close and reopen)
3. Check MCP connection: see settings → Features → Model context protocol

---

## CLI Flags

```
--repo PATH       Path to repository (default: .)
--db PATH         Path to database directory (default: ./rag-data)
--full            Force full re-index (vs incremental)
--version         Show version
```

---

## Performance Expectations

On smart-rag repository (30 Go files, 3659 lines):
- Cold index: ~122ms
- Incremental: ~10ms
- Search query: median 135µs, p95 228µs
- Projected for 1000 files: ~4s index time

---

## Next Steps

1. Try tools in Claude Desktop
2. Create shortcuts for frequently used queries
3. Use `impact_analysis` to understand change blast radius
4. Use `get_context_pack` to retrieve full context before refactoring

Full documentation: https://github.com/robby031/smart-rag
