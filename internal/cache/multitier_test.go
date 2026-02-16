// internal/cache/multitier_test.go
package cache

import (
	"context"
	"errors"
	"testing"
	"time"
)

// mockCache implements CacheManager for testing
type mockCache struct {
	entries map[string]*CacheEntry
	getErr  error
	putErr  error
}

func newMockCache() *mockCache {
	return &mockCache{entries: make(map[string]*CacheEntry)}
}

func (m *mockCache) Get(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	entry, ok := m.entries[key.Hash()]
	if !ok {
		return nil, ErrCacheMiss
	}
	return entry, nil
}

func (m *mockCache) Put(ctx context.Context, entry *CacheEntry) error {
	if m.putErr != nil {
		return m.putErr
	}
	m.entries[entry.Key.Hash()] = entry
	return nil
}

func (m *mockCache) Delete(ctx context.Context, key CacheKey) error {
	delete(m.entries, key.Hash())
	return nil
}

func TestMultiTierCache_GetLocalFirst(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "abc123", FilePath: "/test.go"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	// Put in local only
	local.entries[key.Hash()] = entry

	cache := NewMultiTierCache(local, remote, DefaultMultiTierConfig())
	ctx := context.Background()

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Key.FileHash != key.FileHash {
		t.Errorf("Get() FileHash = %s, want %s", got.Key.FileHash, key.FileHash)
	}
}

func TestMultiTierCache_GetFallbackToRemote(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "xyz789", FilePath: "/main.go"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	// Put in remote only
	remote.entries[key.Hash()] = entry

	cache := NewMultiTierCache(local, remote, DefaultMultiTierConfig())
	ctx := context.Background()

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Key.FileHash != key.FileHash {
		t.Errorf("Get() FileHash = %s, want %s", got.Key.FileHash, key.FileHash)
	}

	// Verify local was warmed
	if _, ok := local.entries[key.Hash()]; !ok {
		t.Error("Get() did not warm local cache")
	}
}

func TestMultiTierCache_GetMiss(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "nonexistent"}

	cache := NewMultiTierCache(local, remote, DefaultMultiTierConfig())
	ctx := context.Background()

	_, err := cache.Get(ctx, key)
	if err != ErrCacheMiss {
		t.Errorf("Get() error = %v, want ErrCacheMiss", err)
	}
}

func TestMultiTierCache_GetRemoteDisabled(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "xyz789"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	// Put in remote only
	remote.entries[key.Hash()] = entry

	config := DefaultMultiTierConfig()
	config.ReadFromRemote = false

	cache := NewMultiTierCache(local, remote, config)
	ctx := context.Background()

	// Should not find it since remote is disabled
	_, err := cache.Get(ctx, key)
	if err != ErrCacheMiss {
		t.Errorf("Get() error = %v, want ErrCacheMiss", err)
	}
}

func TestMultiTierCache_GetRemoteFirst(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "remote-first"}
	localEntry := &CacheEntry{Key: key, Results: nil, Timestamp: 1}
	remoteEntry := &CacheEntry{Key: key, Results: nil, Timestamp: 2}

	local.entries[key.Hash()] = localEntry
	remote.entries[key.Hash()] = remoteEntry

	config := DefaultMultiTierConfig()
	config.PreferLocal = false

	cache := NewMultiTierCache(local, remote, config)
	ctx := context.Background()

	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	// Should get remote entry (timestamp 2)
	if got.Timestamp != 2 {
		t.Errorf("Get() Timestamp = %d, want 2 (remote)", got.Timestamp)
	}
}

func TestMultiTierCache_Put(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "newentry"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	cache := NewMultiTierCache(local, remote, DefaultMultiTierConfig())
	ctx := context.Background()

	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Verify in both caches
	if _, ok := local.entries[key.Hash()]; !ok {
		t.Error("Put() did not write to local cache")
	}
	if _, ok := remote.entries[key.Hash()]; !ok {
		t.Error("Put() did not write to remote cache")
	}
}

func TestMultiTierCache_PutRemoteDisabled(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "localonly"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	config := DefaultMultiTierConfig()
	config.WriteToRemote = false

	cache := NewMultiTierCache(local, remote, config)
	ctx := context.Background()

	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Verify only in local
	if _, ok := local.entries[key.Hash()]; !ok {
		t.Error("Put() did not write to local cache")
	}
	if _, ok := remote.entries[key.Hash()]; ok {
		t.Error("Put() wrote to remote cache when disabled")
	}
}

func TestMultiTierCache_PutRemoteError(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()
	remote.putErr = errors.New("remote error")

	key := CacheKey{FileHash: "remotefails"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	cache := NewMultiTierCache(local, remote, DefaultMultiTierConfig())
	ctx := context.Background()

	// Should succeed despite remote error
	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put() error = %v, want nil (remote error should be logged)", err)
	}

	// Verify local was written
	if _, ok := local.entries[key.Hash()]; !ok {
		t.Error("Put() did not write to local cache")
	}
}

func TestMultiTierCache_Delete(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()

	key := CacheKey{FileHash: "todelete"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	local.entries[key.Hash()] = entry
	remote.entries[key.Hash()] = entry

	cache := NewMultiTierCache(local, remote, DefaultMultiTierConfig())
	ctx := context.Background()

	if err := cache.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted from both
	if _, ok := local.entries[key.Hash()]; ok {
		t.Error("Delete() did not remove from local cache")
	}
	if _, ok := remote.entries[key.Hash()]; ok {
		t.Error("Delete() did not remove from remote cache")
	}
}

func TestMultiTierCache_NoRemote(t *testing.T) {
	local := newMockCache()

	key := CacheKey{FileHash: "localonly"}
	entry := &CacheEntry{Key: key, Timestamp: time.Now().Unix()}

	// Create with nil remote
	cache := NewMultiTierCache(local, nil, DefaultMultiTierConfig())
	ctx := context.Background()

	// Put should work
	if err := cache.Put(ctx, entry); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Get should work
	got, err := cache.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if got.Key.FileHash != key.FileHash {
		t.Errorf("Get() FileHash = %s, want %s", got.Key.FileHash, key.FileHash)
	}

	// HasRemote should be false
	if cache.HasRemote() {
		t.Error("HasRemote() = true, want false")
	}
}

func TestMultiTierCache_Accessors(t *testing.T) {
	local := newMockCache()
	remote := newMockCache()
	config := DefaultMultiTierConfig()

	cache := NewMultiTierCache(local, remote, config)

	if cache.Local() != local {
		t.Error("Local() returned wrong cache")
	}
	if cache.Remote() != remote {
		t.Error("Remote() returned wrong cache")
	}
	if cache.Config() != config {
		t.Error("Config() returned wrong config")
	}
	if !cache.HasRemote() {
		t.Error("HasRemote() = false, want true")
	}
}

func TestDefaultMultiTierConfig(t *testing.T) {
	config := DefaultMultiTierConfig()

	if !config.WriteToRemote {
		t.Error("WriteToRemote should be true by default")
	}
	if !config.ReadFromRemote {
		t.Error("ReadFromRemote should be true by default")
	}
	if !config.PreferLocal {
		t.Error("PreferLocal should be true by default")
	}
	if !config.WarmLocalOnRemoteHit {
		t.Error("WarmLocalOnRemoteHit should be true by default")
	}
}
