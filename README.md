# Smart RAG

RAG-based code intelligence engine with MCP server. Index your codebase and query it via natural language, symbol search, call graph, and impact analysis.

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

- `rag_status` — health check for version, index, graph, BM25, paths, and last sync
- `search_code` — ranked BM25 code search with stable tie-breakers and filters
- `find_definition` — go-to-definition for a symbol
- `find_references` — find all usages of a symbol
- `get_callers` / `get_callees` — call graph navigation
- `impact_analysis` — analyze change impact
- `context_pack` — retrieve relevant code context
- `read_snippet` — read file snippet by path and line range

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
- `VERSION=x.y.z` — binary version (default: `0.3.2`)

## Performance

Benchmarked on the smart-rag repository itself (30 Go files, 3659 lines).

```
smart-rag performance matrix
================================
  Version       : 0.3.2
  Repository    : /Users/bagusdwiharianto/Development/go/smart-rag
  Go files      : 43 (6059 lines)
  Chunks        : 335
  Graph nodes   : 248
  Graph edges   : 948
  Index time    : 139ms
--------------------------------
  Metric                      Target        Actual
  Cold index (  43 files)  ok  < 5-8s       139ms
  Projected   (1000 files) ok  < 5-8s       ~3.239s  [from 43 files]
  Incremental (    1 file) ok  < 1-2s       132ms
  Query search             ok  < 50-80ms    median 137µs     p95 370µs  [335 chunks]
  Query find-def           ok  < 50-80ms    median 1ms      p95 1ms  [335 chunks]
  Query callers            ok  < 50-80ms    median < 1µs     p95 < 1µs  [335 chunks]
  Binary size              ok  < 15-20 MB   6.9 MB
  RAM during index         ok  < 80-120 MB  2.3 MB heap delta
  Query 100k docs          warn  ~20-40ms     ~41ms projected  [linear from 335 chunks]
```
