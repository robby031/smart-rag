BINARY   ?= rag-mcp
VERSION  ?= 0.1.0

.PHONY: all build install clean test

all: build

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) ./cmd/rag-mcp

build-all:
	go build -ldflags="-s -w" ./...

install: build
	@echo "Install $(BINARY) to GOBIN..."
	@go install -ldflags="-s -w -X main.version=$(VERSION)" ./cmd/rag-mcp
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

test:
	go test ./...

clean:
	go clean -cache
	rm -rf ./rag-data $(BINARY)
