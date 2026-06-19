package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/robby031/smart-rag/pkg/instrument"
)

type fileTarget struct {
	path     string
	backup   *fileBackup
	modified bool
}

func discoverGoFiles(repoDir, pkgPattern string) ([]string, error) {
	var files []string
	err := filepath.Walk(repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			name := info.Name()
			if name != "." && strings.HasPrefix(name, ".") {
				return filepath.SkipDir
			}
			if name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(path, "_test.go") && strings.Contains(path, pkgPattern) {
			files = append(files, path)
		}
		if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") && strings.Contains(path, pkgPattern) {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}

func instrumentFiles(files []string, pkg string) ([]*fileTarget, error) {
	inst := instrument.NewInstrumenter()
	var targets []*fileTarget

	for _, path := range files {
		src, err := os.ReadFile(path)
		if err != nil {
			return targets, fmt.Errorf("read %s: %w", path, err)
		}

		bak, err := backupFile(path)
		if err != nil {
			return targets, err
		}

		instrumented, err := inst.Instrument(string(src), path, pkg, "")
		if err != nil {
			bak.restore()
			fmt.Fprintf(os.Stderr, "warning: instrument %s: %v (skipping)\n", path, err)
			continue
		}

		if instrumented == string(src) {
			bak.restore()
			os.Remove(bak.bakPath)
			continue
		}

		if err := os.WriteFile(path, []byte(instrumented), 0644); err != nil {
			bak.restore()
			return targets, fmt.Errorf("write %s: %w", path, err)
		}

		targets = append(targets, &fileTarget{
			path:     path,
			backup:   bak,
			modified: true,
		})
	}

	return targets, nil
}
