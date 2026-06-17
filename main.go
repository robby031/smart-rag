package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/robby031/smart-rag/pkg/engine"
	"github.com/robby031/smart-rag/pkg/indexer"
	"github.com/robby031/smart-rag/pkg/mcp"
	"github.com/robby031/smart-rag/pkg/storage"
)

var version = "dev"

func main() {
	repoDir := flag.String("repo", ".", "Path to the code repository to index")
	dbDir := flag.String("db", "./rag-data", "Path to store the RAG database")
	fullReindex := flag.Bool("full", false, "Force full re-index instead of incremental")
	showVersion := flag.Bool("version", false, "Show version")
	flag.Parse()

	if *showVersion {
		fmt.Printf("rag-mcp %s\n", version)
		os.Exit(0)
	}

	absRepo, err := filepath.Abs(*repoDir)
	if err != nil {
		log.Fatalf("Failed to resolve repo path: %v", err)
	}
	absDB, err := filepath.Abs(*dbDir)
	if err != nil {
		log.Fatalf("Failed to resolve db path: %v", err)
	}
	if err := os.MkdirAll(absDB, 0755); err != nil {
		log.Fatalf("Failed to create db directory: %v", err)
	}

	kvStore, err := storage.OpenStore(filepath.Join(absDB, "kv"))
	if err != nil {
		log.Fatalf("Failed to open KV store: %v", err)
	}
	defer kvStore.Close()

	chunkStore := storage.NewChunkStore(kvStore)
	graphStore := storage.NewGraphStore(kvStore)
	indexStore := storage.NewIndexStore(kvStore)

	vectorDB, err := storage.NewVectorDB(filepath.Join(absDB, "vectors"))
	if err != nil {
		log.Fatalf("Failed to open vector DB: %v", err)
	}

	eng := engine.New(kvStore, chunkStore, vectorDB, graphStore)

	if *fullReindex {
		fmt.Println("Full re-indexing repository:", absRepo)
		if err := eng.IndexDir(context.Background(), absRepo, 0); err != nil {
			log.Fatalf("Failed to index repository: %v", err)
		}
		if err := eng.FinalizeIndex(); err != nil {
			log.Fatalf("Failed to finalize index: %v", err)
		}
	} else {
		syncer := indexer.NewSyncer(eng, indexStore, absRepo)
		indexed, deleted, err := syncer.Sync(context.Background())
		if err != nil {
			log.Fatalf("Failed to sync repository: %v", err)
		}
		fmt.Printf("Incremental indexing: %d files indexed, %d files removed\n", indexed, deleted)
	}

	fmt.Println("Starting smart-rag MCP server...")
	server := mcp.NewServer(eng)
	if err := server.Serve("stdio"); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}
