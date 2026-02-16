// internal/cache/local.go
package cache

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

var cacheTracer = otel.Tracer("github.com/chris-regnier/gavel/internal/cache")

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
	ctx, span := cacheTracer.Start(ctx, "cache lookup")
	defer span.End()

	cacheKeyHash := key.Hash()
	span.SetAttributes(attribute.String("gavel.cache.key", cacheKeyHash))

	// Check context cancellation before I/O
	if err := ctx.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	path := c.entryPath(key)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			span.SetAttributes(attribute.Bool("gavel.cache.hit", false))
			return nil, ErrCacheMiss
		}
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	var entry CacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	span.SetAttributes(attribute.Bool("gavel.cache.hit", true))
	return &entry, nil
}

func (c *LocalCache) Put(ctx context.Context, entry *CacheEntry) error {
	_, span := cacheTracer.Start(ctx, "cache store")
	defer span.End()

	span.SetAttributes(attribute.String("gavel.cache.key", entry.Key.Hash()))

	// Check context cancellation before I/O
	if err := ctx.Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err := os.MkdirAll(c.dir, 0755); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	entry.Timestamp = time.Now().Unix()
	data, err := json.MarshalIndent(entry, "", "  ")
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if err := os.WriteFile(c.entryPath(entry.Key), data, 0644); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (c *LocalCache) Delete(ctx context.Context, key CacheKey) error {
	// Check context cancellation before I/O
	if err := ctx.Err(); err != nil {
		return err
	}

	return os.Remove(c.entryPath(key))
}
