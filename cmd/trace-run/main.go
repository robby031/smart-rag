package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
)

func main() {
	repoDir := flag.String("repo", ".", "Path ke repository")
	pkgPattern := flag.String("pkg", "./...", "Package pattern untuk instrumentasi")
	testPattern := flag.String("test", "./...", "Test pattern untuk dijalankan")
	doCleanup := flag.Bool("cleanup", true, "Bersihkan file instrumentasi setelah selesai")
	flag.Parse()

	absRepo, err := filepath.Abs(*repoDir)
	if err != nil {
		log.Fatalf("Failed to resolve repo path: %v", err)
	}

	fmt.Fprintf(os.Stderr, "Discovering Go files in %s matching %s...\n", absRepo, *pkgPattern)
	files, err := discoverGoFiles(absRepo, *pkgPattern)
	if err != nil {
		log.Fatalf("File discovery: %v", err)
	}
	if len(files) == 0 {
		log.Fatalf("No Go files found matching pattern %s", *pkgPattern)
	}
	fmt.Fprintf(os.Stderr, "Found %d Go files\n", len(files))

	pkgName := *pkgPattern
	if after, ok := strings.CutPrefix(pkgName, "./"); ok {
		pkgName = after
	}
	pkgName = strings.ReplaceAll(pkgName, "/", ".")
	pkgName = strings.TrimSuffix(pkgName, "...")
	pkgName = strings.Trim(pkgName, ".")

	fmt.Fprintf(os.Stderr, "Instrumenting files...\n")
	targets, err := instrumentFiles(files, pkgName)
	if err != nil {
		log.Fatalf("Instrumentation: %v", err)
	}
	fmt.Fprintf(os.Stderr, "Instrumented %d files\n", len(targets))

	var backups []*fileBackup
	for _, t := range targets {
		backups = append(backups, t.backup)
	}

	if *doCleanup {
		defer cleanup(backups)
	}

	fmt.Fprintf(os.Stderr, "Running tests: go test -v %s\n", *testPattern)
	runner := newTraceRunner(absRepo, *testPattern)
	output, err := runner.run()
	fmt.Println(output)
	if err != nil {
		log.Printf("Test run completed with errors: %v", err)
	}

	events := runner.collectEvents()
	fmt.Fprintf(os.Stderr, "\n=== Trace Events (%d total) ===\n", len(events))
	printEvents(events)
}
