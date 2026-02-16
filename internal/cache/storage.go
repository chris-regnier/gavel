// internal/cache/storage.go
package cache

import (
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Storage provides an abstraction for storing and retrieving cache data
type Storage interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Put(ctx context.Context, key string, data []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// LocalStorage implements Storage using the local filesystem
type LocalStorage struct {
	dir string
}

// NewLocalStorage creates a new local filesystem storage
func NewLocalStorage(dir string) *LocalStorage {
	return &LocalStorage{dir: dir}
}

// keyPath converts a cache key to a filesystem path
func (s *LocalStorage) keyPath(key string) string {
	return filepath.Join(s.dir, key+".json")
}

// Get retrieves data for the given key
func (s *LocalStorage) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	data, err := os.ReadFile(s.keyPath(key))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrCacheMiss
		}
		return nil, err
	}
	return data, nil
}

// Put stores data for the given key
func (s *LocalStorage) Put(ctx context.Context, key string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return err
	}

	return os.WriteFile(s.keyPath(key), data, 0644)
}

// Delete removes data for the given key
func (s *LocalStorage) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}

	err := os.Remove(s.keyPath(key))
	if err != nil && os.IsNotExist(err) {
		return nil // Not an error if already deleted
	}
	return err
}

// List returns all keys matching the given prefix
func (s *LocalStorage) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var keys []string
	err := filepath.WalkDir(s.dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		// Extract key from path (remove dir prefix and .json suffix)
		relPath, err := filepath.Rel(s.dir, path)
		if err != nil {
			return err
		}
		key := strings.TrimSuffix(relPath, ".json")
		if prefix == "" || strings.HasPrefix(key, prefix) {
			keys = append(keys, key)
		}
		return nil
	})

	if err != nil && os.IsNotExist(err) {
		return nil, nil // Empty list if directory doesn't exist
	}
	return keys, err
}

// Dir returns the storage directory path
func (s *LocalStorage) Dir() string {
	return s.dir
}
