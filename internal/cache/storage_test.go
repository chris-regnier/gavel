// internal/cache/storage_test.go
package cache

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLocalStorage_PutAndGet(t *testing.T) {
	dir := t.TempDir()
	storage := NewLocalStorage(dir)
	ctx := context.Background()

	key := "test-key"
	data := []byte(`{"test": "data"}`)

	// Put data
	if err := storage.Put(ctx, key, data); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Get data
	got, err := storage.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}

	if string(got) != string(data) {
		t.Errorf("Get() = %s, want %s", got, data)
	}
}

func TestLocalStorage_GetMissing(t *testing.T) {
	dir := t.TempDir()
	storage := NewLocalStorage(dir)
	ctx := context.Background()

	_, err := storage.Get(ctx, "nonexistent")
	if err != ErrCacheMiss {
		t.Errorf("Get() error = %v, want ErrCacheMiss", err)
	}
}

func TestLocalStorage_Delete(t *testing.T) {
	dir := t.TempDir()
	storage := NewLocalStorage(dir)
	ctx := context.Background()

	key := "test-key"
	data := []byte(`{"test": "data"}`)

	// Put data
	if err := storage.Put(ctx, key, data); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Delete data
	if err := storage.Delete(ctx, key); err != nil {
		t.Fatalf("Delete() error = %v", err)
	}

	// Verify deleted
	_, err := storage.Get(ctx, key)
	if err != ErrCacheMiss {
		t.Errorf("Get() after delete error = %v, want ErrCacheMiss", err)
	}
}

func TestLocalStorage_DeleteNonexistent(t *testing.T) {
	dir := t.TempDir()
	storage := NewLocalStorage(dir)
	ctx := context.Background()

	// Delete nonexistent key should not error
	if err := storage.Delete(ctx, "nonexistent"); err != nil {
		t.Errorf("Delete() error = %v, want nil", err)
	}
}

func TestLocalStorage_List(t *testing.T) {
	dir := t.TempDir()
	storage := NewLocalStorage(dir)
	ctx := context.Background()

	// Put multiple keys
	keys := []string{"key1", "key2", "prefix-a", "prefix-b"}
	for _, k := range keys {
		if err := storage.Put(ctx, k, []byte("data")); err != nil {
			t.Fatalf("Put(%s) error = %v", k, err)
		}
	}

	// List all
	all, err := storage.List(ctx, "")
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(all) != 4 {
		t.Errorf("List() returned %d keys, want 4", len(all))
	}

	// List with prefix
	prefixed, err := storage.List(ctx, "prefix")
	if err != nil {
		t.Fatalf("List(prefix) error = %v", err)
	}
	if len(prefixed) != 2 {
		t.Errorf("List(prefix) returned %d keys, want 2", len(prefixed))
	}
}

func TestLocalStorage_ListEmptyDir(t *testing.T) {
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "nonexistent")
	storage := NewLocalStorage(nonexistent)
	ctx := context.Background()

	keys, err := storage.List(ctx, "")
	if err != nil {
		t.Errorf("List() error = %v, want nil", err)
	}
	if keys != nil && len(keys) != 0 {
		t.Errorf("List() returned %v, want empty", keys)
	}
}

func TestLocalStorage_ContextCancellation(t *testing.T) {
	dir := t.TempDir()
	storage := NewLocalStorage(dir)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Operations should fail with context error
	if _, err := storage.Get(ctx, "key"); err != context.Canceled {
		t.Errorf("Get() error = %v, want context.Canceled", err)
	}

	if err := storage.Put(ctx, "key", []byte("data")); err != context.Canceled {
		t.Errorf("Put() error = %v, want context.Canceled", err)
	}

	if err := storage.Delete(ctx, "key"); err != context.Canceled {
		t.Errorf("Delete() error = %v, want context.Canceled", err)
	}

	if _, err := storage.List(ctx, ""); err != context.Canceled {
		t.Errorf("List() error = %v, want context.Canceled", err)
	}
}

func TestLocalStorage_Dir(t *testing.T) {
	dir := "/some/path"
	storage := NewLocalStorage(dir)
	if storage.Dir() != dir {
		t.Errorf("Dir() = %s, want %s", storage.Dir(), dir)
	}
}

func TestLocalStorage_CreatesDirectory(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "subdir", "cache")
	storage := NewLocalStorage(subdir)
	ctx := context.Background()

	// Put should create the directory
	if err := storage.Put(ctx, "key", []byte("data")); err != nil {
		t.Fatalf("Put() error = %v", err)
	}

	// Verify directory was created
	if _, err := os.Stat(subdir); os.IsNotExist(err) {
		t.Error("Put() did not create directory")
	}
}
