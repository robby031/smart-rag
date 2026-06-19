package main

import (
	"fmt"
	"os"
)

type fileBackup struct {
	origPath string
	bakPath  string
	content  []byte
}

func backupFile(path string) (*fileBackup, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	bakPath := path + ".tracebak"
	if err := os.WriteFile(bakPath, content, 0644); err != nil {
		return nil, fmt.Errorf("backup %s: %w", path, err)
	}
	return &fileBackup{origPath: path, bakPath: bakPath, content: content}, nil
}

func (b *fileBackup) restore() error {
	return os.WriteFile(b.origPath, b.content, 0644)
}

func cleanup(backups []*fileBackup) {
	for _, b := range backups {
		if err := b.restore(); err != nil {
			fmt.Fprintf(os.Stderr, "warning: restore %s: %v\n", b.origPath, err)
		}
		os.Remove(b.bakPath)
	}
}
