// internal/cache/remote.go
package cache

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

// RemoteCache implements CacheManager using a remote HTTP cache server
type RemoteCache struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

// RemoteCacheOption configures a RemoteCache
type RemoteCacheOption func(*RemoteCache)

// WithHTTPClient sets a custom HTTP client
func WithHTTPClient(client *http.Client) RemoteCacheOption {
	return func(c *RemoteCache) {
		c.httpClient = client
	}
}

// WithToken sets the authentication token
func WithToken(token string) RemoteCacheOption {
	return func(c *RemoteCache) {
		c.token = token
	}
}

// WithTimeout sets the HTTP client timeout
func WithTimeout(timeout time.Duration) RemoteCacheOption {
	return func(c *RemoteCache) {
		c.httpClient.Timeout = timeout
	}
}

// NewRemoteCache creates a new remote cache client
func NewRemoteCache(baseURL string, opts ...RemoteCacheOption) *RemoteCache {
	c := &RemoteCache{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c
}

// Get retrieves a cache entry from the remote server
func (c *RemoteCache) Get(ctx context.Context, key CacheKey) (*CacheEntry, error) {
	hash := key.Hash()
	reqURL := fmt.Sprintf("%s/api/cache/%s", c.baseURL, url.PathEscape(hash))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, ErrCacheMiss
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var entry CacheEntry
	if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &entry, nil
}

// Put stores a cache entry on the remote server
func (c *RemoteCache) Put(ctx context.Context, entry *CacheEntry) error {
	hash := entry.Key.Hash()
	reqURL := fmt.Sprintf("%s/api/cache/%s", c.baseURL, url.PathEscape(hash))

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encoding entry: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL, bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Delete removes a cache entry from the remote server
func (c *RemoteCache) Delete(ctx context.Context, key CacheKey) error {
	hash := key.Hash()
	reqURL := fmt.Sprintf("%s/api/cache/%s", c.baseURL, url.PathEscape(hash))

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	// 404 is acceptable for delete - entry might already be gone
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// Stats retrieves cache statistics from the remote server
func (c *RemoteCache) Stats(ctx context.Context) (*RemoteCacheStats, error) {
	reqURL := fmt.Sprintf("%s/api/cache/stats", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("executing request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, string(body))
	}

	var stats RemoteCacheStats
	if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}

	return &stats, nil
}

// Ping checks if the remote server is reachable
func (c *RemoteCache) Ping(ctx context.Context) error {
	reqURL := fmt.Sprintf("%s/api/health", c.baseURL)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	c.setAuthHeader(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("server unreachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// setAuthHeader adds the authentication header if a token is configured
func (c *RemoteCache) setAuthHeader(req *http.Request) {
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
}

// RemoteCacheStats holds statistics from the remote cache server
type RemoteCacheStats struct {
	Entries   int64   `json:"entries"`
	SizeBytes int64   `json:"size_bytes"`
	HitRate   float64 `json:"hit_rate"`
	Hits      int64   `json:"hits"`
	Misses    int64   `json:"misses"`
}
