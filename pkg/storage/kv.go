package storage

import (
	"fmt"
	"time"

	"go.etcd.io/bbolt"
)

type Store struct {
	db *bbolt.DB
}

func OpenStore(path string) (*Store, error) {
	db, err := bbolt.Open(path+".db", 0600, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("boltdb open: %w", err)
	}

	err = db.Update(func(tx *bbolt.Tx) error {
		_, err := tx.CreateBucketIfNotExists([]byte("rag"))
		return err
	})
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("boltdb init bucket: %w", err)
	}

	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) Get(key []byte) ([]byte, error) {
	var val []byte
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		v := b.Get(key)
		if v == nil {
			return ErrNotFound
		}
		val = make([]byte, len(v))
		copy(val, v)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return val, nil
}

func (s *Store) Put(key, value []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		return b.Put(key, value)
	})
}

type KVPair struct {
	Key   []byte
	Value []byte
}

func (s *Store) BatchPut(pairs []KVPair) error {
	if len(pairs) == 0 {
		return nil
	}
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		for _, p := range pairs {
			if err := b.Put(p.Key, p.Value); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) Delete(key []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		return b.Delete(key)
	})
}

func (s *Store) Sync() error {
	return s.db.Sync()
}

var ErrNotFound = fmt.Errorf("key not found")

func (s *Store) GetWithPrefix(prefix []byte) (map[string][]byte, error) {
	result := make(map[string][]byte)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		c := b.Cursor()
		for k, v := c.Seek(prefix); k != nil && len(k) >= len(prefix); k, v = c.Next() {
			if string(k[:len(prefix)]) != string(prefix) {
				break
			}
			val := make([]byte, len(v))
			copy(val, v)
			result[string(k)] = val
		}
		return nil
	})
	return result, err
}

func (s *Store) HasPrefix(prefix []byte) (bool, error) {
	var found bool
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		c := b.Cursor()
		k, _ := c.Seek(prefix)
		found = k != nil && len(k) >= len(prefix) && string(k[:len(prefix)]) == string(prefix)
		return nil
	})
	return found, err
}

func (s *Store) DeleteWithPrefix(prefix []byte) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("rag"))
		c := b.Cursor()

		var keys [][]byte
		for k, _ := c.Seek(prefix); k != nil && len(k) >= len(prefix); k, _ = c.Next() {
			if string(k[:len(prefix)]) != string(prefix) {
				break
			}
			kCopy := make([]byte, len(k))
			copy(kCopy, k)
			keys = append(keys, kCopy)
		}

		for _, k := range keys {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}
