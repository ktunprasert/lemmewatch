package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"
)

func TestCacheExpiresLazily(t *testing.T) {
	storage := openTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	storage.now = func() time.Time { return now }
	if err := storage.CachePut(CacheSearch, "dune", []string{"tt1"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	var value []string
	hit, err := storage.CacheGet(CacheSearch, "dune", &value)
	if err != nil || !hit || len(value) != 1 || value[0] != "tt1" {
		t.Fatalf("cache hit = %t, %#v, %v", hit, value, err)
	}
	now = now.Add(time.Hour)
	hit, err = storage.CacheGet(CacheSearch, "dune", &value)
	if err != nil || hit {
		t.Fatalf("expired cache hit = %t, %v", hit, err)
	}
	if err := storage.cache.View(func(tx *bolt.Tx) error {
		if bucket := tx.Bucket(cacheBucket(CacheSearch)); bucket != nil && bucket.Get([]byte("dune")) != nil {
			t.Fatal("expired entry was not deleted")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestCacheReplacementUpdatesExpiry(t *testing.T) {
	storage := openTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	storage.now = func() time.Time { return now }
	if err := storage.CachePut(CacheSeries, "tt1", "old", time.Hour); err != nil {
		t.Fatal(err)
	}
	now = now.Add(30 * time.Minute)
	if err := storage.CachePut(CacheSeries, "tt1", "new", time.Hour); err != nil {
		t.Fatal(err)
	}
	now = now.Add(45 * time.Minute)
	var value string
	hit, err := storage.CacheGet(CacheSeries, "tt1", &value)
	if err != nil || !hit || value != "new" {
		t.Fatalf("replacement = %t, %q, %v", hit, value, err)
	}
}

func TestCacheWriteSweepsExpiredEntries(t *testing.T) {
	storage := openTestStorage(t)
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	storage.now = func() time.Time { return now }
	if err := storage.CachePut(CacheSearch, "old", "value", time.Hour); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Hour)
	if err := storage.CachePut(CacheSearch, "new", "value", time.Hour); err != nil {
		t.Fatal(err)
	}
	var value string
	hit, err := storage.CacheGet(CacheSearch, "old", &value)
	if err != nil || hit {
		t.Fatalf("swept cache hit = %t, %v", hit, err)
	}
}

func TestStorageCreatesPrivateDatabaseFiles(t *testing.T) {
	root := t.TempDir()
	historyPath := filepath.Join(root, "config", "history.db")
	cachePath := filepath.Join(root, "cache", "cache.db")
	storage := NewAt(historyPath, cachePath, filepath.Join(root, "history.json"))
	if err := storage.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	for _, path := range []string{historyPath, cachePath} {
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("database path %q: %v", path, err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("database mode = %o", info.Mode().Perm())
		}
	}
}

type blockingValue struct {
	started chan struct{}
	release chan struct{}
}

func (value *blockingValue) UnmarshalJSON([]byte) error {
	close(value.started)
	<-value.release
	return nil
}

func TestCloseWaitsForActiveCacheRead(t *testing.T) {
	storage := openTestStorage(t)
	if err := storage.CachePut(CacheSearch, "key", "value", time.Hour); err != nil {
		t.Fatal(err)
	}
	value := blockingValue{started: make(chan struct{}), release: make(chan struct{})}
	readDone := make(chan error, 1)
	go func() {
		_, err := storage.CacheGet(CacheSearch, "key", &value)
		readDone <- err
	}()
	<-value.started
	closeDone := make(chan error, 1)
	go func() { closeDone <- storage.Close() }()
	select {
	case err := <-closeDone:
		t.Fatalf("storage closed during cache read: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(value.release)
	if err := <-readDone; err != nil {
		t.Fatal(err)
	}
	if err := <-closeDone; err != nil {
		t.Fatal(err)
	}
}
