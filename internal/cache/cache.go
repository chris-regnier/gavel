// internal/cache/cache.go
package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"

	"github.com/chris-regnier/gavel/internal/sarif"
)

var ErrCacheMiss = errors.New("cache miss")

// CacheKey identifies a unique analysis result
type CacheKey struct {
	FileHash    string            `json:"file_hash"`
	FilePath    string            `json:"file_path"`
	Provider    string            `json:"provider"`
	Model       string            `json:"model"`
	BAMLVersion string            `json:"baml_version"`
	Policies    map[string]string `json:"policies"` // policy name -> instruction hash
}

// Hash computes deterministic cache key
func (k CacheKey) Hash() string {
	b, err := json.Marshal(k)
	if err != nil {
		// CacheKey is a simple struct that should always marshal successfully
		panic("failed to marshal CacheKey: " + err.Error())
	}
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// CacheEntry represents a cached analysis result
type CacheEntry struct {
	Key       CacheKey       `json:"key"`
	Results   []sarif.Result `json:"results"`
	Timestamp int64          `json:"timestamp"`
}

// CacheManager provides cached analysis results
type CacheManager interface {
	Get(ctx context.Context, key CacheKey) (*CacheEntry, error)
	Put(ctx context.Context, entry *CacheEntry) error
	Delete(ctx context.Context, key CacheKey) error
}
