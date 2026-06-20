package storage

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"
)

const (
	indexHashPrefix = "idx:hash:"
	indexMetaKey    = "idx:meta"
)

type IndexMeta struct {
	LastUpdated   time.Time `json:"last_updated"`
	FileCount     int       `json:"file_count"`
	TotalChunks   int       `json:"total_chunks"`
	IndexedCommit string    `json:"indexed_commit,omitempty"`
}

type IndexStore struct {
	kv *Store
}

func NewIndexStore(kv *Store) *IndexStore {
	return &IndexStore{kv: kv}
}

func ContentHash(content []byte) string {
	h := sha1.Sum(content)
	return hex.EncodeToString(h[:])
}

func (is *IndexStore) SaveHash(filePath, hash string) error {
	return is.kv.Put([]byte(indexHashPrefix+filePath), []byte(hash))
}

func (is *IndexStore) GetHash(filePath string) (string, error) {
	data, err := is.kv.Get([]byte(indexHashPrefix + filePath))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (is *IndexStore) HasChanged(filePath string, content []byte) (bool, error) {
	stored, err := is.GetHash(filePath)
	if err != nil {
		return true, nil
	}
	return stored != ContentHash(content), nil
}

func (is *IndexStore) DeleteHash(filePath string) error {
	return is.kv.Delete([]byte(indexHashPrefix + filePath))
}

func (is *IndexStore) SaveMeta(meta IndexMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal index meta: %w", err)
	}
	return is.kv.Put([]byte(indexMetaKey), data)
}

func (is *IndexStore) LoadMeta() (*IndexMeta, error) {
	data, err := is.kv.Get([]byte(indexMetaKey))
	if err != nil {
		return nil, err
	}
	var meta IndexMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal index meta: %w", err)
	}
	return &meta, nil
}
