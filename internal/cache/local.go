// internal/cache/local.go
package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Ensure LocalCache implements CacheManager interface
var _ CacheManager = (*LocalCache)(nil)

type LocalCache struct {
	dir string
}

func NewLocalCache(dir string) *LocalCache {
	return &LocalCache{dir: dir}
}

func (c *LocalCache) entryPath(key CacheKey) string {
	return filepath.Join(c.dir, key.Hash()+".json")
}

func (c *LocalCache) Get(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	path := c.entryPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (c *LocalCache) Put(ctx context.Context, entry *CacheEntry) error {
	if err := os.MkdirAll(c.dir, 0755); err != nil {
		return err
	}

	entry.Timestamp = time.Now().Unix()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(c.entryPath(entry.Key), data, 0644)
}

func (c *LocalCache) Delete(ctx context.Context, key CacheKey) error {
	return os.Remove(c.entryPath(key))
}
