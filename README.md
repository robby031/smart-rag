# Smart RAG

RAG-based code intelligence engine with MCP server. Index your codebase and query it via natural language, symbol search, call graph, and impact analysis.

**Supported languages:** Go, JavaScript, TypeScript (including JSX, TSX, ES modules)

## Architecture

![smart-rag architecture](docs/architecture.svg)

## Quick Start

### Auto Installation with AI

Copy the setup guide URL and share with your AI assistant:
```
https://raw.githubusercontent.com/robby031/smart-rag/main/docs/SETUP_PROMPT.md
```

Paste this link to Claude, ChatGPT, or any AI and say:
> "Read this setup guide and help me install smart-rag on my system"

The AI will read the guide and step you through the entire installation process for your device.

---

### Manual Setup

[📖 **Setup Guide for AI Assistants**](docs/SETUP_PROMPT.md) — Step-by-step instructions for all three setup methods.

---

## Usage

### Build

```bash
make build
```

### Run (incremental index + MCP server)

```bash
make run REPO=/path/to/your/project
```

### Run (full re-index)

```bash
make run-full REPO=/path/to/your/project
```

### CLI flags

| Flag       | Default  | Description                         |
| ---------- | -------- | ----------------------------------- |
| `--repo`   | `.`      | Path to the code repository to index |
| `--db`     | `./rag-data` | Path to store the RAG database    |
| `--full`   | `false`  | Force full re-index instead of incremental |
| `--pruning`| `soft`   | Index pruning mode: `off`, `soft`, or `hard` |
| `--version`| `false`  | Show version                        |

### Docker Hub (no build required)

**1. Pull image**

```bash
docker pull robbymangku/smart-rag:latest
```

**2. Index your repo**

```bash
docker run --rm \
  -v "/path/to/your/project:/repo:ro" \
  -v "smart-rag-data:/data" \
  robbymangku/smart-rag:latest --repo=/repo --db=/data --full
```

**3. Add to Claude Desktop** — `~/Library/Application Support/Claude/claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "smart-rag": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/your/project:/repo:ro",
        "-v", "smart-rag-data:/data",
        "robbymangku/smart-rag:latest"
      ]
    }
  }
}
```

**4. Add to Claude Code** — `.mcp.json` in your project root:

```json
{
  "mcpServers": {
    "smart-rag": {
      "command": "docker",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/your/project:/repo:ro",
        "-v", "smart-rag-data:/data",
        "robbymangku/smart-rag:latest"
      ]
    }
  }
}
```

Restart Claude after adding the config. On each new session, Docker automatically runs an incremental sync before the MCP server starts.

**Update to the latest version:**

```bash
docker pull robbymangku/smart-rag:latest
```

> Also available on GHCR: `ghcr.io/robby031/smart-rag:latest`

---

### Docker (build locally)

**1. Build image**

```bash
make docker-build
```

**2. Index your repo**

```bash
make docker-index REPO=/path/to/your/project
```

**3.** Use the same MCP config as above, replacing the image with `smart-rag:latest`.

**Re-index after updating smart-rag source:**

```bash
make docker-restart REPO=/path/to/your/project
```

---

### Binary (without Docker)

Run `make install` or add to your MCP client config:

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

### Available MCP Tools

| Tool | Description |
|---|---|
| `rag_status` | Health check — version, index, graph, BM25, paths, and last sync |
| `reindex` | Trigger incremental reindex without restarting the server. Call this after a large refactor or when search results seem stale |
| `search_code` | Ranked BM25 code search with stable tie-breakers, language filter, and path filter |
| `find_definition` | Go-to-definition for a symbol (Go and JS/TS: functions, classes, types, enums, interfaces) |
| `find_references` | Find all usages of a symbol across the codebase |
| `get_callers` | List all functions that call the given function |
| `get_callees` | List all functions called by the given function |
| `impact_analysis` | Analyze blast radius of changing a function or package across the call/import graph |
| `get_context_pack` | Retrieve full code context for a chunk, budget-limited for AI consumption |
| `read_snippet` | Read a file snippet by path and line range (e.g. `main.go:10-25`) |
| `trace_variable` | Trace a variable through its def-use chain — where defined, modified, and used |
| `function_dataflow` | Show data flow inside a function: inputs, internal variables, and return values |
| `type_flow` | Trace how a type is used across the codebase (forward and backward) |
| `variable_search` | Semantic search for variables — exact, fuzzy, or type-based |
| `trace_runtime` | Show runtime trace data for a function or variable collected via test instrumentation |

### Make targets

```bash
make build        # Build production binary
make run          # Incremental index + serve
make run-full     # Full re-index + serve
make test         # Run tests
make clean        # Remove artifacts
```

### Configuration

- `REPO=path` — source repository (default: `.`)
- `DB=path` — database directory (default: `./rag-data`)
- `PRUNING=off|soft|hard` — index pruning mode (default: `soft`)
- `VERSION=x.y.z` — binary version (default: `0.4.7`)

`--pruning` maps to the index pruning setting `index.pruning.mode`.

Pruning modes:

- `off` — keep all indexed chunks and ignore pruning metadata during search/context assembly.
- `soft` — default; keep all chunks, lower search weight for unreachable/boilerplate chunks, and skip them from automatic context expansion.
- `hard` — delete unreachable/boilerplate chunks from the chunk store after indexing, then rebuild BM25 from remaining chunks.

## Performance

Benchmarked smart-rag

```
smart-rag performance benchmark
═══════════════════════════════════════════════════════════════
  Version      : 0.4.7
  Repository   : /Users/bagusdwiharianto/Development/ai/smart-rag
  Source files : 87  (15642 lines)
  Pruning      : soft

  Index Stats
  ───────────────────────────────────────────────────────────
  Chunks       : 843
  Graph nodes  : 650
  Graph edges  : 2734

  Indexing Performance
  ───────────────────────────────────────────────────────────
  Full index         : 1.704s        (87 files)
  Per file (avg)     : 19.583ms
  Incremental 1-file : 119ms
  No-op sync         : 13ms
  Heap delta         : 3.7 MB
  Binary size        : 10.3 MB

  Query Latency
  ───────────────────────────────────────────────────────────
  Operation            Queries   Median     P95        P99        Min        Max
  search                    450   13ms       13ms       14ms       496µs      27ms
  search+filter             150   13ms       14ms       14ms       12ms       14ms
  find_definition           300   4ms        4ms        4ms        3ms        4ms
  find_references           240   1ms        2ms        2ms        1ms        2ms
  get_callers               240   < 1µs      < 1µs      < 1µs      < 1µs      2µs
  get_callees               240   < 1µs      < 1µs      < 1µs      < 1µs      < 1µs
  impact_analysis           120   < 1µs      < 1µs      1µs        < 1µs      1µs
  get_context_pack          160   5ms        5ms        5ms        4ms        5ms
  read_snippet              300   42µs       104µs      155µs      9µs        204µs
```
