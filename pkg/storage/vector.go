package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/philippgille/chromem-go"
)

type VectorDB struct {
	db   *chromem.DB
	coll *chromem.Collection
}

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

func (cs *ChunkStore) GetAllByFile(filePath string) ([]*ChunkMeta, error) {
	raw, err := cs.kv.GetWithPrefix([]byte("chunk:" + filePath))
	if err != nil {
		return nil, err
	}
	chunks := make([]*ChunkMeta, 0, len(raw))
	for _, data := range raw {
		var meta ChunkMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			return nil, fmt.Errorf("unmarshal chunk meta: %w", err)
		}
		chunks = append(chunks, &meta)
	}
	sort.SliceStable(chunks, func(i, j int) bool {
		if chunks[i].StartLine != chunks[j].StartLine {
			return chunks[i].StartLine < chunks[j].StartLine
		}
		if chunks[i].EndLine != chunks[j].EndLine {
			return chunks[i].EndLine < chunks[j].EndLine
		}
		return chunks[i].ID < chunks[j].ID
	})
	return chunks, nil
}

func (cs *ChunkStore) DeleteByFile(filePath string) error {
	return cs.kv.DeleteWithPrefix([]byte("chunk:" + filePath))
}

func (cs *ChunkStore) SearchBySymbol(query string, chunkTypes []string) ([]*ChunkMeta, error) {
	queryLower := strings.ToLower(query)
	typeSet := make(map[string]bool, len(chunkTypes))
	for _, t := range chunkTypes {
		typeSet[t] = true
	}
	raw, err := cs.kv.GetWithPrefix([]byte("chunk:"))
	if err != nil {
		return nil, err
	}
	var results []*ChunkMeta
	for _, data := range raw {
		var meta ChunkMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		if !typeSet[meta.ChunkType] {
			continue
		}
		if !strings.Contains(strings.ToLower(meta.SymbolName), queryLower) {
			continue
		}
		m := meta
		results = append(results, &m)
	}
	return results, nil
}

func (cs *ChunkStore) GetAll() ([]*ChunkMeta, error) {
	raw, err := cs.kv.GetWithPrefix([]byte("chunk:"))
	if err != nil {
		return nil, err
	}
	chunks := make([]*ChunkMeta, 0, len(raw))
	for _, data := range raw {
		var meta ChunkMeta
		if err := json.Unmarshal(data, &meta); err != nil {
			continue
		}
		chunks = append(chunks, &meta)
	}
	return chunks, nil
}

func (cs *ChunkStore) PutAll(metas []ChunkMeta) error {
	pairs := make([]KVPair, 0, len(metas))
	for _, meta := range metas {
		data, err := json.Marshal(meta)
		if err != nil {
			return fmt.Errorf("marshal chunk meta %s: %w", meta.ID, err)
		}
		pairs = append(pairs, KVPair{Key: []byte("chunk:" + meta.ID), Value: data})
	}
	return cs.kv.BatchPut(pairs)
}
