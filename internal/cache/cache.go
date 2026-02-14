package cache

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/chris-regnier/gavel/internal/sarif"
)

// Entry represents a cached analysis result for the in-memory cache
type Entry struct {
	Key       string
	Value     interface{}
	CreatedAt time.Time
	ExpiresAt time.Time
	HitCount  int64
}

// IsExpired returns true if the entry has expired
func (e *Entry) IsExpired() bool {
	if e.ExpiresAt.IsZero() {
		return false
	}
	return time.Now().After(e.ExpiresAt)
}

// Cache provides a thread-safe in-memory cache with TTL support
type Cache struct {
	mu      sync.RWMutex
	entries map[string]*Entry
	maxSize int
	ttl     time.Duration

	// Stats
	hits      int64
	misses    int64
	evictions int64
}

// Option configures a Cache
type Option func(*Cache)

// WithMaxSize sets the maximum number of entries
func WithMaxSize(n int) Option {
	return func(c *Cache) {
		c.maxSize = n
	}
}

// WithTTL sets the default time-to-live for entries
func WithTTL(d time.Duration) Option {
	return func(c *Cache) {
		c.ttl = d
	}
}

// New creates a new cache with the given options
func New(opts ...Option) *Cache {
	c := &Cache{
		entries: make(map[string]*Entry),
		maxSize: 1000,
		ttl:     1 * time.Hour,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Get retrieves a value from the cache
// Returns the value and true if found and not expired, nil and false otherwise
func (c *Cache) Get(key string) (interface{}, bool) {
	c.mu.RLock()
	entry, ok := c.entries[key]
	c.mu.RUnlock()

	if !ok {
		c.mu.Lock()
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	if entry.IsExpired() {
		c.mu.Lock()
		delete(c.entries, key)
		c.misses++
		c.mu.Unlock()
		return nil, false
	}

	c.mu.Lock()
	entry.HitCount++
	c.hits++
	c.mu.Unlock()

	return entry.Value, true
}

// Set stores a value in the cache with the default TTL
func (c *Cache) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.ttl)
}

// SetWithTTL stores a value in the cache with a custom TTL
func (c *Cache) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict if at capacity
	if len(c.entries) >= c.maxSize {
		c.evictOldest()
	}

	now := time.Now()
	var expiresAt time.Time
	if ttl > 0 {
		expiresAt = now.Add(ttl)
	}

	c.entries[key] = &Entry{
		Key:       key,
		Value:     value,
		CreatedAt: now,
		ExpiresAt: expiresAt,
		HitCount:  0,
	}
}

// Delete removes an entry from the cache
func (c *Cache) Delete(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, key)
}

// Clear removes all entries from the cache
func (c *Cache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries = make(map[string]*Entry)
}

// Size returns the current number of entries
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}

// Stats returns cache statistics
func (c *Cache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()

	total := c.hits + c.misses
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(c.hits) / float64(total)
	}

	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		HitRate:   hitRate,
		Size:      len(c.entries),
		MaxSize:   c.maxSize,
		Evictions: c.evictions,
	}
}

// CacheStats holds cache statistics
type CacheStats struct {
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
	HitRate   float64 `json:"hit_rate"`
	Size      int     `json:"size"`
	MaxSize   int     `json:"max_size"`
	Evictions int64   `json:"evictions"`
}

// evictOldest removes the oldest entry (by creation time)
// Must be called with lock held
func (c *Cache) evictOldest() {
	var oldestKey string
	var oldestTime time.Time

	for key, entry := range c.entries {
		if oldestKey == "" || entry.CreatedAt.Before(oldestTime) {
			oldestKey = key
			oldestTime = entry.CreatedAt
		}
	}

	if oldestKey != "" {
		delete(c.entries, oldestKey)
		c.evictions++
	}
}

// Cleanup removes all expired entries
func (c *Cache) Cleanup() int {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for key, entry := range c.entries {
		if entry.IsExpired() {
			delete(c.entries, key)
			count++
		}
	}
	return count
}

// GenerateKey creates a cache key from multiple components
func GenerateKey(components ...string) string {
	h := sha256.New()
	for i, comp := range components {
		if i > 0 {
			h.Write([]byte{0}) // separator
		}
		h.Write([]byte(comp))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ContentKey creates a cache key specifically for code analysis
// combining content, policies, and persona
func ContentKey(content, policies, persona string) string {
	return GenerateKey(content, policies, persona)
}

// ============================================================================
// LSP CacheManager interface and types
// ============================================================================

var ErrCacheMiss = errors.New("cache miss")

// CacheKey identifies a unique analysis result for LSP caching
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

// CacheEntry represents a cached analysis result for LSP
type CacheEntry struct {
	Key       CacheKey       `json:"key"`
	Results   []sarif.Result `json:"results"`
	Timestamp int64          `json:"timestamp"`
}

// CacheManager provides cached analysis results for LSP
type CacheManager interface {
	Get(ctx context.Context, key CacheKey) (*CacheEntry, error)
	Put(ctx context.Context, entry *CacheEntry) error
	Delete(ctx context.Context, key CacheKey) error
}
