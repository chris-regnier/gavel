package cache

import (
	"sync"
	"testing"
	"time"
)

func TestCache_SetGet(t *testing.T) {
	c := New()
	
	c.Set("key1", "value1")
	
	val, ok := c.Get("key1")
	if !ok {
		t.Fatal("expected to find key1")
	}
	if val != "value1" {
		t.Errorf("expected value1, got %v", val)
	}
}

func TestCache_GetMiss(t *testing.T) {
	c := New()
	
	val, ok := c.Get("nonexistent")
	if ok {
		t.Error("expected cache miss")
	}
	if val != nil {
		t.Errorf("expected nil value, got %v", val)
	}
	
	stats := c.Stats()
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
}

func TestCache_TTL(t *testing.T) {
	c := New(WithTTL(50 * time.Millisecond))
	
	c.Set("key1", "value1")
	
	// Should exist immediately
	if _, ok := c.Get("key1"); !ok {
		t.Error("expected to find key1 immediately")
	}
	
	// Wait for expiry
	time.Sleep(60 * time.Millisecond)
	
	// Should be expired now
	if _, ok := c.Get("key1"); ok {
		t.Error("expected key1 to be expired")
	}
}

func TestCache_CustomTTL(t *testing.T) {
	c := New(WithTTL(1 * time.Hour)) // Default long TTL
	
	// Set with short TTL
	c.SetWithTTL("short", "value", 50*time.Millisecond)
	c.Set("long", "value")
	
	time.Sleep(60 * time.Millisecond)
	
	if _, ok := c.Get("short"); ok {
		t.Error("expected short-ttl entry to be expired")
	}
	if _, ok := c.Get("long"); !ok {
		t.Error("expected long-ttl entry to still exist")
	}
}

func TestCache_MaxSize(t *testing.T) {
	c := New(WithMaxSize(3))
	
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Set("key3", "value3")
	
	if c.Size() != 3 {
		t.Errorf("expected size 3, got %d", c.Size())
	}
	
	// Adding a 4th should evict the oldest
	c.Set("key4", "value4")
	
	if c.Size() != 3 {
		t.Errorf("expected size 3 after eviction, got %d", c.Size())
	}
	
	stats := c.Stats()
	if stats.Evictions != 1 {
		t.Errorf("expected 1 eviction, got %d", stats.Evictions)
	}
}

func TestCache_Delete(t *testing.T) {
	c := New()
	
	c.Set("key1", "value1")
	c.Delete("key1")
	
	if _, ok := c.Get("key1"); ok {
		t.Error("expected key1 to be deleted")
	}
}

func TestCache_Clear(t *testing.T) {
	c := New()
	
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.Clear()
	
	if c.Size() != 0 {
		t.Errorf("expected size 0 after clear, got %d", c.Size())
	}
}

func TestCache_Concurrent(t *testing.T) {
	c := New()
	
	var wg sync.WaitGroup
	n := 100
	
	// Concurrent writes
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Set(string(rune('a'+i%26)), i)
		}(i)
	}
	
	// Concurrent reads
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			c.Get(string(rune('a' + i%26)))
		}(i)
	}
	
	wg.Wait()
	
	// Should not panic and stats should be consistent
	stats := c.Stats()
	if stats.Hits+stats.Misses == 0 {
		t.Error("expected some cache operations")
	}
}

func TestCache_HitRate(t *testing.T) {
	c := New()
	
	c.Set("key1", "value1")
	
	// 3 hits
	c.Get("key1")
	c.Get("key1")
	c.Get("key1")
	
	// 1 miss
	c.Get("nonexistent")
	
	stats := c.Stats()
	if stats.Hits != 3 {
		t.Errorf("expected 3 hits, got %d", stats.Hits)
	}
	if stats.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", stats.Misses)
	}
	
	expectedRate := 0.75
	if stats.HitRate < expectedRate-0.01 || stats.HitRate > expectedRate+0.01 {
		t.Errorf("expected hit rate ~%.2f, got %.2f", expectedRate, stats.HitRate)
	}
}

func TestCache_Cleanup(t *testing.T) {
	c := New(WithTTL(50 * time.Millisecond))
	
	c.Set("key1", "value1")
	c.Set("key2", "value2")
	c.SetWithTTL("key3", "value3", 1*time.Hour) // Won't expire
	
	time.Sleep(60 * time.Millisecond)
	
	removed := c.Cleanup()
	
	if removed != 2 {
		t.Errorf("expected 2 entries removed, got %d", removed)
	}
	
	if c.Size() != 1 {
		t.Errorf("expected 1 entry remaining, got %d", c.Size())
	}
}

func TestGenerateKey(t *testing.T) {
	key1 := GenerateKey("a", "b", "c")
	key2 := GenerateKey("a", "b", "c")
	key3 := GenerateKey("a", "b", "d")
	key4 := GenerateKey("ab", "c")
	
	if key1 != key2 {
		t.Error("same inputs should produce same key")
	}
	if key1 == key3 {
		t.Error("different inputs should produce different keys")
	}
	if key1 == key4 {
		t.Error("different component boundaries should produce different keys")
	}
}

func TestContentKey(t *testing.T) {
	content := "package main\n\nfunc main() {}\n"
	policies := "- error-handling [warning]: Check errors\n"
	persona := "code-reviewer"
	
	key1 := ContentKey(content, policies, persona)
	key2 := ContentKey(content, policies, persona)
	
	if key1 != key2 {
		t.Error("same content should produce same key")
	}
	
	// Change content slightly
	key3 := ContentKey(content+" ", policies, persona)
	if key1 == key3 {
		t.Error("different content should produce different key")
	}
}

func TestEntry_IsExpired(t *testing.T) {
	// Entry with zero time (never expires)
	e1 := &Entry{ExpiresAt: time.Time{}}
	if e1.IsExpired() {
		t.Error("zero-time entry should not be expired")
	}
	
	// Entry in the past
	e2 := &Entry{ExpiresAt: time.Now().Add(-1 * time.Hour)}
	if !e2.IsExpired() {
		t.Error("past entry should be expired")
	}
	
	// Entry in the future
	e3 := &Entry{ExpiresAt: time.Now().Add(1 * time.Hour)}
	if e3.IsExpired() {
		t.Error("future entry should not be expired")
	}
}

func BenchmarkCache_Set(b *testing.B) {
	c := New()
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		c.Set(string(rune(i%1000)), i)
	}
}

func BenchmarkCache_Get(b *testing.B) {
	c := New()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		c.Set(string(rune(i)), i)
	}
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		c.Get(string(rune(i % 1000)))
	}
}

func BenchmarkCache_SetGetParallel(b *testing.B) {
	c := New()
	
	// Pre-populate
	for i := 0; i < 1000; i++ {
		c.Set(string(rune(i)), i)
	}
	
	b.ReportAllocs()
	b.ResetTimer()
	
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%2 == 0 {
				c.Set(string(rune(i%1000)), i)
			} else {
				c.Get(string(rune(i % 1000)))
			}
			i++
		}
	})
}

func BenchmarkGenerateKey(b *testing.B) {
	content := "package main\n\nfunc main() {}\n"
	policies := "- error-handling [warning]: Check errors\n"
	persona := "code-reviewer"
	
	b.ReportAllocs()
	b.ResetTimer()
	
	for i := 0; i < b.N; i++ {
		_ = ContentKey(content, policies, persona)
	}
}
