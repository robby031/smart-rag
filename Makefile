BINARY   ?= rag-mcp
BENCH    ?= rag-mcp-bench
DB       ?= ./rag-data
REPO     ?= .
VERSION  ?= 0.4.7
IMAGE    ?= smart-rag
PRUNING  ?= soft

.PHONY: build bench run run-full install test test-race vet bench-run \
        docker-build docker-run docker-index docker-restart clean help

build:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BINARY) .

bench:
	go build -ldflags="-s -w -X main.version=$(VERSION)" -o $(BENCH) ./cmd/bench

run: build
	./$(BINARY) --repo "$(REPO)" --db "$(DB)" --pruning "$(PRUNING)"

run-full: build
	./$(BINARY) --repo "$(REPO)" --db "$(DB)" --full --pruning "$(PRUNING)"

bench-run: bench
	./$(BENCH) --repo "$(REPO)" --pruning "$(PRUNING)"

test:
	go test ./...

test-race:
	go test -race ./...

vet:
	go vet ./...

install:
	go install -ldflags="-s -w -X main.version=$(VERSION)" .

docker-build:
	docker build --build-arg VERSION=$(VERSION) -t $(IMAGE):latest .

docker-run:
	docker run -i --rm \
	  -v "$(abspath $(REPO)):/repo:ro" \
	  -v "smart-rag-data:/data" \
	  $(IMAGE):latest

docker-index:
	REPO_DIR=$(REPO) docker compose run --rm index

docker-restart: docker-build
	REPO_DIR=$(REPO) docker compose run --rm index

clean:
	rm -f $(BINARY) $(BENCH)
	rm -rf $(DB)

help:
	@echo "Usage: make <target> [VAR=value]"
	@echo ""
	@echo "Build & Run"
	@echo "  build       Build MCP server binary"
	@echo "  run         Start MCP server (incremental index)"
	@echo "  run-full    Start MCP server (full re-index)"
	@echo "  install     Install binary to GOPATH/bin"
	@echo ""
	@echo "Benchmark"
	@echo "  bench       Build benchmark binary"
	@echo "  bench-run   Run performance benchmark"
	@echo ""
	@echo "Quality"
	@echo "  test        Run tests"
	@echo "  test-race   Run tests with race detector"
	@echo "  vet         Run go vet"
	@echo ""
	@echo "Docker"
	@echo "  docker-build    Build Docker image"
	@echo "  docker-run      Run MCP server in Docker"
	@echo "  docker-index    Full re-index via Docker Compose"
	@echo "  docker-restart  Rebuild image + re-index"
	@echo ""
	@echo "Variables"
	@echo "  REPO=path       Repository to index   (default: .)"
	@echo "  DB=path         Database directory     (default: ./rag-data)"
	@echo "  PRUNING=mode    off, soft, or hard     (default: soft)"
	@echo "  VERSION=x.y.z   Binary version         (default: 0.4.7)"
	@echo "  IMAGE=name      Docker image name      (default: smart-rag)"
	@echo ""
	@echo "Examples"
	@echo "  make run REPO=~/myproject"
	@echo "  make bench-run REPO=~/myproject"
	@echo "  make docker-run REPO=~/myproject"
