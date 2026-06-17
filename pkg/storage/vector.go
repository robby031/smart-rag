package storage

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/philippgille/chromem-go"
)

type VectorDB struct {
	db   *chromem.DB
	coll *chromem.Collection
}

// NewVectorDB creates a chromem-go vector DB. Empty persistPath = in-memory.
func NewVectorDB(persistPath string) (*VectorDB, error) {
	var db *chromem.DB
	var err error

	if persistPath != "" {
		db, err = chromem.NewPersistentDB(persistPath, false)
	} else {
		db = chromem.NewDB()
	}
	if err != nil {
		return nil, fmt.Errorf("chromem new db: %w", err)
	}

	coll, err := db.CreateCollection("code_chunks", nil, nil)
	if err != nil {
		return nil, fmt.Errorf("chromem create collection: %w", err)
	}

	return &VectorDB{db: db, coll: coll}, nil
}

func (vdb *VectorDB) AddDocument(ctx context.Context, id string, embedding []float32, metadata map[string]string) error {
	return vdb.coll.AddDocument(ctx, chromem.Document{
		ID:        id,
		Embedding: embedding,
		Metadata:  metadata,
	})
}

func (vdb *VectorDB) Search(ctx context.Context, embedding []float32, topK int) ([]chromem.Result, error) {
	return vdb.coll.QueryEmbedding(ctx, embedding, topK, nil, nil)
}

func (vdb *VectorDB) Count() int {
	return vdb.coll.Count()
}

type ChunkMeta struct {
	ID         string `json:"id"`
	FilePath   string `json:"file_path"`
	ChunkType  string `json:"chunk_type"`
	SymbolName string `json:"symbol_name,omitempty"`
	Signature  string `json:"signature,omitempty"`
	StartLine  int    `json:"start_line"`
	EndLine    int    `json:"end_line"`
	Content    string `json:"content"`
}

type ChunkStore struct {
	kv *Store
}

func NewChunkStore(kv *Store) *ChunkStore {
	return &ChunkStore{kv: kv}
}

func (cs *ChunkStore) Put(meta ChunkMeta) error {
	data, err := json.Marshal(meta)
	if err != nil {
		return fmt.Errorf("marshal chunk meta: %w", err)
	}
	return cs.kv.Put([]byte("chunk:"+meta.ID), data)
}

func (cs *ChunkStore) Get(id string) (*ChunkMeta, error) {
	data, err := cs.kv.Get([]byte("chunk:" + id))
	if err != nil {
		return nil, err
	}
	var meta ChunkMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, fmt.Errorf("unmarshal chunk meta: %w", err)
	}
	return &meta, nil
}

func (cs *ChunkStore) Delete(id string) error {
	return cs.kv.Delete([]byte("chunk:" + id))
}
