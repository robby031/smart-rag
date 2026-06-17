# Smart RAG

RAG-based code intelligence engine with MCP server. Index your codebase and query it via natural language, symbol search, call graph, and impact analysis.

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

### VS Code MCP Integration

Run `make install` or add to VS Code `settings.json`:

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
- `VERSION=x.y.z` — binary version (default: `0.1.0`)
