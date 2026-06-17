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

### Docker via GHCR (no build required)

**1. Pull image**

```bash
docker pull ghcr.io/robby031/smart-rag:latest
```

**2. Index your repo**

```bash
docker run --rm \
  -v "/path/to/your/project:/repo:ro" \
  -v "smart-rag-data:/data" \
  ghcr.io/robby031/smart-rag:latest --repo=/repo --db=/data --full
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
        "ghcr.io/robby031/smart-rag:latest"
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
        "ghcr.io/robby031/smart-rag:latest"
      ]
    }
  }
}
```

Restart Claude after adding the config. On each new session, Docker automatically runs an incremental sync before the MCP server starts.

**Update to the latest version:**

```bash
docker pull ghcr.io/robby031/smart-rag:latest
```

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

- `search_code` — hybrid search (BM25 + sparse vector)
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
- `VERSION=x.y.z` — binary version (default: `0.2.0`)

## Performance

Benchmarked on the smart-rag repository itself (30 Go files, 3659 lines).

```
smart-rag performance matrix
================================
  Version       : 0.2.0
  Repository    : /Users/bagusdwiharianto/Development/go/smart-rag
  Go files      : 30 (3659 lines)
  Chunks        : 247
  Graph nodes   : 165
  Graph edges   : 539
  Index time    : 122ms
--------------------------------
  Metric                      Target        Actual
  Cold index (  30 files)  ok  < 5-8s       122ms
  Projected   (1000 files) ok  < 5-8s       ~4.078s  [from 30 files]
  Incremental (    1 file) ok  < 1-2s       10ms
  Query search             ok  < 50-80ms    median 135µs     p95 228µs  [247 chunks]
  Query find-def           ok  < 50-80ms    median 67µs      p95 77µs  [247 chunks]
  Query callers            ok  < 50-80ms    median < 1µs     p95 < 1µs  [247 chunks]
  Binary size              ok  < 15-20 MB   6.8 MB
  RAM during index         ok  < 80-120 MB  1.7 MB heap delta
  Query 100k docs          warn  ~20-40ms     ~55ms projected  [linear from 247 chunks]
```
