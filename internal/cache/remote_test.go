// internal/cache/remote_test.go
package cache

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRemoteCache_Get(t *testing.T) {
	entry := &CacheEntry{
		Key: CacheKey{
			FileHash: "abc123",
			FilePath: "/test.go",
			Provider: "openrouter",
			Model:    "claude-3",
		},
		Results:   nil,
		Timestamp: time.Now().Unix(),
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("Expected GET, got %s", r.Method)
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Error("Expected Accept: application/json header")
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entry)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	got, err := cache.Get(ctx, entry.Key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if got.Key.FileHash != entry.Key.FileHash {
		t.Errorf("Get().Key.FileHash = %s, want %s", got.Key.FileHash, entry.Key.FileHash)
	}
}

func TestRemoteCache_Get_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	_, err := cache.Get(ctx, CacheKey{FileHash: "missing"})
	if err != ErrCacheMiss {
		t.Errorf("Get() error = %v, want ErrCacheMiss", err)
	}
}

func TestRemoteCache_Put(t *testing.T) {
	var receivedEntry CacheEntry
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("Expected PUT, got %s", r.Method)
		}
		if r.Header.Get("Content-Type") != "application/json" {
			t.Error("Expected Content-Type: application/json header")
		}

		json.NewDecoder(r.Body).Decode(&receivedEntry)
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	entry := &CacheEntry{
		Key: CacheKey{
			FileHash: "xyz789",
			FilePath: "/main.go",
		},
		Timestamp: time.Now().Unix(),
	}

	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	if receivedEntry.Key.FileHash != entry.Key.FileHash {
		t.Errorf("Server received FileHash = %s, want %s", receivedEntry.Key.FileHash, entry.Key.FileHash)
	}
}

func TestRemoteCache_Delete(t *testing.T) {
	deleted := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("Expected DELETE, got %s", r.Method)
		}
		deleted = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	if err := cache.Delete(ctx, CacheKey{FileHash: "todelete"}); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	if !deleted {
		t.Error("Delete() did not send request to server")
	}
}

func TestRemoteCache_Delete_NotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	// Delete of nonexistent should succeed
	if err := cache.Delete(ctx, CacheKey{FileHash: "nonexistent"}); err != nil {
		t.Errorf("Delete() error = %v, want nil", err)
	}
}

func TestRemoteCache_WithToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-token" {
			t.Errorf("Authorization = %s, want Bearer test-token", auth)
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL, WithToken("test-token"))
	ctx := context.Background()

	cache.Get(ctx, CacheKey{FileHash: "any"})
}

func TestRemoteCache_Ping(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/health" {
			t.Errorf("Expected /api/health, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	if err := cache.Ping(ctx); err != nil {
		t.Errorf("Ping() error = %v", err)
	}
}

func TestRemoteCache_Ping_Failure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	if err := cache.Ping(ctx); err == nil {
		t.Error("Ping() expected error for non-200 response")
	}
}

func TestRemoteCache_Stats(t *testing.T) {
	stats := &RemoteCacheStats{
		Entries:   100,
		SizeBytes: 1024000,
		HitRate:   0.85,
		Hits:      850,
		Misses:    150,
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cache/stats" {
			t.Errorf("Expected /api/cache/stats, got %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)
	ctx := context.Background()

	got, err := cache.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats() error = %v", err)
	}

	if got.Entries != stats.Entries {
		t.Errorf("Stats().Entries = %d, want %d", got.Entries, stats.Entries)
	}
	if got.HitRate != stats.HitRate {
		t.Errorf("Stats().HitRate = %f, want %f", got.HitRate, stats.HitRate)
	}
}

func TestRemoteCache_ContextCancellation(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	cache := NewRemoteCache(server.URL)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	_, err := cache.Get(ctx, CacheKey{FileHash: "any"})
	if err == nil {
		t.Error("Get() expected error for cancelled context")
	}
}
