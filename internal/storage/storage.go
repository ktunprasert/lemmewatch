package storage

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

const lockTimeout = 250 * time.Millisecond

var ErrSessionActive = errors.New("another Lemmewatch session is active; close it and try again")

type Storage struct {
	historyPath string
	cachePath   string
	legacyPath  string
	now         func() time.Time

	mu       sync.Mutex
	history  *bolt.DB
	cache    *bolt.DB
	cacheErr error
}

func New() *Storage {
	configRoot, err := os.UserConfigDir()
	if err != nil || configRoot == "" {
		configRoot = "."
	}
	cacheRoot, err := os.UserCacheDir()
	if err != nil || cacheRoot == "" {
		cacheRoot = configRoot
	}
	return NewAt(
		filepath.Join(configRoot, "lemmewatch", "history.db"),
		filepath.Join(cacheRoot, "lemmewatch", "cache.db"),
		filepath.Join(configRoot, "lemmewatch", "history.json"),
	)
}

func NewAt(historyPath, cachePath, legacyPath string) *Storage {
	return &Storage{historyPath: historyPath, cachePath: cachePath, legacyPath: legacyPath, now: time.Now}
}

func (s *Storage) Open() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.history != nil || s.cache != nil {
		return nil
	}

	history, err := open(s.historyPath)
	if err != nil {
		return storageOpenError(err)
	}
	s.history = history
	if err := s.migrateLegacyHistory(); err != nil {
		s.history.Close()
		s.history = nil
		return fmt.Errorf("migrate history: %w", err)
	}

	cache, err := open(s.cachePath)
	if err != nil {
		s.cacheErr = storageOpenError(err)
		return nil
	}
	s.cache = cache
	s.cacheErr = nil
	return nil
}

func open(path string) (*bolt.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	return bolt.Open(path, 0o600, &bolt.Options{Timeout: lockTimeout})
}

func storageOpenError(err error) error {
	if errors.Is(err, bolt.ErrTimeout) {
		return ErrSessionActive
	}
	return fmt.Errorf("open storage: %w", err)
}

func (s *Storage) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var errs []error
	if s.cache != nil {
		errs = append(errs, s.cache.Close())
		s.cache = nil
	}
	if s.history != nil {
		errs = append(errs, s.history.Close())
		s.history = nil
	}
	return errors.Join(errs...)
}

func (s *Storage) CacheError() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cacheErr
}

func SourceFingerprint(source string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(source)))
}
