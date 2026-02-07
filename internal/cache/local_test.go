// internal/cache/local_test.go
package cache

import (
	"context"
	"testing"

	"github.com/chris-regnier/gavel/internal/sarif"
)

func TestLocalCacheGetMiss(t *testing.T) {
	dir := t.TempDir()
	cache := NewLocalCache(dir)

	key := CacheKey{FileHash: "abc123", Provider: "ollama", Model: "test"}
	_, err := cache.Get(context.Background(), key)
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss, got %v", err)
	}
}

func TestLocalCachePutGet(t *testing.T) {
	dir := t.TempDir()
	cache := NewLocalCache(dir)

	key := CacheKey{
		FileHash: "abc123",
		Provider: "ollama",
		Model:    "test",
		Policies: map[string]string{"policy1": "instruction-hash"},
	}
	entry := &CacheEntry{
		Key: key,
		Results: []sarif.Result{
			{RuleID: "test-rule", Level: "error", Message: sarif.Message{Text: "test"}},
		},
	}

	ctx := context.Background()
	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if len(got.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(got.Results))
	}
	if got.Results[0].RuleID != "test-rule" {
		t.Errorf("expected rule ID test-rule, got %s", got.Results[0].RuleID)
	}
}

func TestLocalCacheDelete(t *testing.T) {
	dir := t.TempDir()
	cache := NewLocalCache(dir)

	key := CacheKey{
		FileHash: "delete-test",
		Provider: "ollama",
		Model:    "test",
	}
	entry := &CacheEntry{
		Key: key,
		Results: []sarif.Result{
			{RuleID: "test-rule", Level: "error", Message: sarif.Message{Text: "test"}},
		},
	}

	ctx := context.Background()

	// Put entry
	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put failed: %v", err)
	}

	// Verify it exists
	if _, err := cache.Get(ctx, key); err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	// Delete entry
	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Verify it's gone
	_, err := cache.Get(ctx, key)
	if err != ErrCacheMiss {
		t.Fatalf("expected ErrCacheMiss after delete, got %v", err)
	}
}
