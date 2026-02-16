// internal/cache/multitier.go
package cache

import (
	"context"
	"log"
)

// MultiTierConfig configures the multi-tier cache behavior
type MultiTierConfig struct {
	// WriteToRemote controls whether to write cache entries to the remote server
	WriteToRemote bool

	// ReadFromRemote controls whether to read from the remote server on local miss
	ReadFromRemote bool

	// PreferLocal controls whether to check local cache first (true) or remote first (false)
	PreferLocal bool

	// WarmLocalOnRemoteHit controls whether to populate local cache on remote hit
	WarmLocalOnRemoteHit bool
}

// DefaultMultiTierConfig returns the default multi-tier cache configuration
func DefaultMultiTierConfig() MultiTierConfig {
	return MultiTierConfig{
		WriteToRemote:        true,
		ReadFromRemote:       true,
		PreferLocal:          true,
		WarmLocalOnRemoteHit: true,
	}
}

// MultiTierCache implements CacheManager with local and optional remote tiers
type MultiTierCache struct {
	local  CacheManager
	remote CacheManager // may be nil
	config MultiTierConfig
}

// NewMultiTierCache creates a new multi-tier cache
// If remote is nil, the cache operates in local-only mode
func NewMultiTierCache(local CacheManager, remote CacheManager, config MultiTierConfig) *MultiTierCache {
	return &MultiTierCache{
		local:  local,
		remote: remote,
		config: config,
	}
}

// Get retrieves a cache entry, checking local and remote tiers based on config
func (c *MultiTierCache) Get(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	if c.config.PreferLocal {
		return c.getLocalFirst(ctx, key)
	}
	return c.getRemoteFirst(ctx, key)
}

// getLocalFirst checks local cache first, then falls back to remote
func (c *MultiTierCache) getLocalFirst(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	// Try local first
	entry, err := c.local.Get(ctx, key)
	if err == nil {
		return entry, nil
	}

	// Fall back to remote if enabled and available
	if !c.config.ReadFromRemote || c.remote == nil {
		return nil, ErrCacheMiss
	}

	entry, err = c.remote.Get(ctx, key)
	if err != nil {
		return nil, err
	}

	// Warm local cache on remote hit
	if c.config.WarmLocalOnRemoteHit {
		if putErr := c.local.Put(ctx, entry); putErr != nil {
			log.Printf("Failed to warm local cache: %v", putErr)
		}
	}

	return entry, nil
}

// getRemoteFirst checks remote cache first, then falls back to local
func (c *MultiTierCache) getRemoteFirst(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	// Try remote first if enabled and available
	if c.config.ReadFromRemote && c.remote != nil {
		entry, err := c.remote.Get(ctx, key)
		if err == nil {
			// Warm local cache on remote hit
			if c.config.WarmLocalOnRemoteHit {
				if putErr := c.local.Put(ctx, entry); putErr != nil {
					log.Printf("Failed to warm local cache: %v", putErr)
				}
			}
			return entry, nil
		}
	}

	// Fall back to local
	return c.local.Get(ctx, key)
}

// Put stores a cache entry in local and optionally remote caches
func (c *MultiTierCache) Put(ctx context.Context, entry *CacheEntry) error {
	// Always write to local
	if err := c.local.Put(ctx, entry); err != nil {
		return err
	}

	// Write to remote if enabled and available
	if c.config.WriteToRemote && c.remote != nil {
		if err := c.remote.Put(ctx, entry); err != nil {
			// Log but don't fail - local write succeeded
			log.Printf("Failed to write to remote cache: %v", err)
		}
	}

	return nil
}

// Delete removes a cache entry from local and optionally remote caches
func (c *MultiTierCache) Delete(ctx context.Context, key CacheKey) error {
	// Delete from local
	if err := c.local.Delete(ctx, key); err != nil {
		return err
	}

	// Delete from remote if available
	if c.remote != nil {
		if err := c.remote.Delete(ctx, key); err != nil {
			// Log but don't fail - local delete succeeded
			log.Printf("Failed to delete from remote cache: %v", err)
		}
	}

	return nil
}

// HasRemote returns true if a remote cache is configured
func (c *MultiTierCache) HasRemote() bool {
	return c.remote != nil
}

// Local returns the local cache manager
func (c *MultiTierCache) Local() CacheManager {
	return c.local
}

// Remote returns the remote cache manager (may be nil)
func (c *MultiTierCache) Remote() CacheManager {
	return c.remote
}

// Config returns the current configuration
func (c *MultiTierCache) Config() MultiTierConfig {
	return c.config
}
