BINARY   ?= rag-mcp
BENCH    ?= rag-mcp-bench
DB       ?= ./rag-data
REPO     ?= .
VERSION  ?= 0.1.0

.PHONY: all build bench install run run-full bench-run bench-run-full test clean help

all: build

build:           ; @echo "Build production binary → $(BINARY)"
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) .

build-all:       ; @echo "Build all packages"
	go build -ldflags="-s -w" ./...

bench:           ; @echo "Build benchmark binary → $(BENCH)"
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BENCH) ./cmd/bench

run: build        ; @echo "Run MCP server (incremental index)"
	./$(BINARY) --repo "$(REPO)" --db "$(DB)"

run-full: build   ; @echo "Run MCP server (full re-index)"
	./$(BINARY) --repo "$(REPO)" --db "$(DB)" --full

bench-run: bench  ; @echo "Run benchmark (incremental)"
	./$(BENCH) --repo "$(REPO)" --db "$(DB)"

bench-run-full: bench ; @echo "Run benchmark (full re-index)"
	./$(BENCH) --repo "$(REPO)" --db "$(DB)" --full

install: build
	@echo "Install $(BINARY) to GOBIN..."
	@go install -ldflags="-s -w -X main.version=$(VERSION)" .
	@echo ""
	@echo "Add to VS Code settings.json:"
	@echo '{'
	@echo '  "mcpServers": {'
	@echo '    "rag-codebase": {'
	@echo '      "command": "$(BINARY)",'
	@echo '      "args": ["--repo", "/path/to/your/project"]'
	@echo '    }'
	@echo '  }'
	@echo '}'

test:            ; @echo "Run all tests"
	go test ./...

clean:           ; @echo "Remove build artifacts and database"
	go clean -cache
	rm -rf $(DB) $(BINARY) $(BENCH) bench

help:
	@echo "smart-rag — Make targets"
	@echo "========================"
	@echo ""
	@echo "  build           Build production binary (rag-mcp)"
	@echo "  bench           Build benchmark binary (rag-mcp-bench)"
	@echo "  run             Incremental index + start MCP server"
	@echo "  run-full        Full re-index + start MCP server"
	@echo "  bench-run       Run benchmark (incremental)"
	@echo "  bench-run-full  Run benchmark (full re-index)"
	@echo "  install         Build + install to GOBIN"
	@echo "  test            Run go test"
	@echo "  clean           Remove artifacts and database"
	@echo ""
	@echo "Variables:"
	@echo "  REPO=path       Source repository path (default: .)"
	@echo "  DB=path         Database directory (default: ./rag-data)"
	@echo "  VERSION=x.y.z   Binary version (default: 0.1.0)"
	@echo ""
	@echo "Example:"
	@echo "  make run REPO=/home/user/project DB=~/rag-data"
