# Smart RAG Setup Guide

**Purpose:** Pemandu untuk AI assistant membantu user install smart-rag secara mandiri.

---

## Penjelasan Singkat

Smart RAG adalah code intelligence engine berbasis RAG (Retrieval-Augmented Generation) yang meng-index codebase dan menyediakan MCP tools untuk Claude Desktop / Claude Code. Dengan smart-rag, user bisa:
- Search code dengan natural language (BM25 + sparse vector)
- Go-to-definition (find_definition)
- Find semua referensi symbol (find_references)
- Explore call graph (get_callers / get_callees)
- Impact analysis (blast radius dari perubahan)
- Read code snippet by file:line

Smart RAG berjalan sebagai MCP server yang berkomunikasi via JSON-RPC pada stdio.

---

## Prerequisites

Pilih salah satu setup method sesuai kenyamanan user:

### Method 1: Docker via GHCR (Recommended - No build required)
- Docker installed (`docker --version`)
- Docker can run containers dengan volume mount
- Repository path user (lokal atau remote)

### Method 2: Docker Build Locally
- Docker installed
- Git installed
- Repository path user

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

**Step 2: Index repository (first time)**
```bash
# Ganti /path/to/your/project dengan path actual user
docker run --rm \
  -v "/path/to/your/project:/repo:ro" \
  -v "smart-rag-data:/data" \
  ghcr.io/robby031/smart-rag:latest --repo=/repo --db=/data --full
```

Output akan menampilkan:
```
Full re-indexing repository: /repo
Chunks: XXX
Graph nodes: XXX
Graph edges: XXX
Incremental indexing: XXX files indexed
Starting smart-rag MCP server...
```

Jika error "bind: address already in use", stop container lama:
```bash
docker stop smart-rag
docker rm smart-rag
```

**Step 3: Add ke Claude Desktop**

Buka atau create file:
```
~/Library/Application Support/Claude/claude_desktop_config.json
```

Tambahkan / update `mcpServers` section (ganti path):
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

**Step 4: Add ke Claude Code**

Buat / update file `.mcp.json` di project root (ganti path):
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

**Update image ke latest:**
```bash
docker pull ghcr.io/robby031/smart-rag:latest
```

---

### Method 2: Docker Build Locally

**Step 1: Clone repo**
```bash
git clone https://github.com/robby031/smart-rag.git
cd smart-rag
```

**Step 2: Build image**
```bash
make docker-build
# Output: smart-rag:latest (13.2 MB)
```

**Step 3: Index repo**
```bash
make docker-index REPO=/path/to/your/project
```

**Step 4: Configure MCP** (sama seperti Method 1)

Update config file, ganti image URL dari:
```
ghcr.io/robby031/smart-rag:latest
```
ke:
```
smart-rag:latest
```

**Step 5: Rebuild after update**
```bash
make docker-restart REPO=/path/to/your/project
```

---

### Method 3: Binary Without Docker

**Step 1: Clone repo**
```bash
git clone https://github.com/robby031/smart-rag.git
cd smart-rag
```

**Step 2: Build**
```bash
make build
# Output: ./rag-mcp (binary)
```

**Step 3: Index repo**
```bash
./rag-mcp --repo=/path/to/your/project --db=./rag-data --full
```

**Step 4: Install to GOBIN**
```bash
make install
# Binary akan tersedia di $GOBIN/rag-mcp
```

**Step 5: Configure MCP**

Add ke Claude Desktop / Claude Code config:
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

Setelah setup, test di Claude Desktop:

1. Buka Chat
2. Klik icon tools di bottom → Smart RAG tools akan visible
3. Coba query:
   - `search_code`: "search keyword dalam codebase"
   - `find_definition`: "nama function yang cari definisinya"
   - `get_callers`: "function ID"
   - `impact_analysis`: "function/package name"

Jika tools tidak muncul, cek:
- Config file syntax valid JSON (gunakan jsonlint)
- Path `/path/to/your/project` correct dan readable
- Docker image tersedia: `docker images | grep smart-rag`
- Container logs: `docker logs smart-rag`

---

## Troubleshooting

### Error: "bind: address already in use"
**Solusi:** Ada container lama masih running.
```bash
docker ps | grep smart-rag
docker stop <container-id>
```

### Error: "Unexpected token 'I', 'Incrementa'... is not valid JSON"
**Sebab:** stdout terkontaminasi output non-JSON.
**Solusi:** Pastikan version terbaru (`docker pull ghcr.io/robby031/smart-rag:latest`).

### Error: "chunk not found" / "file not found"
**Sebab:** Re-indexing belum selesai atau path wrong.
**Solusi:** Jalankan index ulang:
```bash
# Method 1 (GHCR)
docker run --rm -v "/path:/repo:ro" -v "smart-rag-data:/data" \
  ghcr.io/robby031/smart-rag:latest --repo=/repo --db=/data --full

# Method 2 (Local)
make docker-restart REPO=/path/to/project

# Method 3 (Binary)
./rag-mcp --repo=/path/to/project --db=./rag-data --full
```

### MCP tools tidak muncul di Claude
**Solusi:**
1. Verify config JSON valid
2. Restart Claude completely (close dan reopen)
3. Check MCP connection: lihat preferences → Features → Model context protocol

---

## CLI Flags

```
--repo PATH       Path ke repository (default: .)
--db PATH         Path ke database directory (default: ./rag-data)
--full            Force full re-index (vs incremental)
--version         Show version
```

---

## Performance Expectations

Pada smart-rag repo (30 Go files, 3659 lines):
- Cold index: ~122ms
- Incremental: ~10ms
- Search query: median 135µs, p95 228µs
- Projected untuk 1000 files: ~4s index time

---

## Next Steps

1. Coba tools di Claude Desktop
2. Create shortcuts untuk queries yang sering dipakai
3. Use `impact_analysis` untuk understand change blast radius
4. Use `get_context_pack` untuk retrieve full context sebelum refactor

Dokumentasi lengkap: https://github.com/robby031/smart-rag
