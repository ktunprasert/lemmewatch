package storage

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	CacheSearch   = "search"
	CacheSeries   = "series"
	CacheTorrents = "torrents"

	expirySweepLimit = 128
)

var expiryBucket = []byte("expiry")

type cacheEntry struct {
	ExpiresAt int64           `json:"expires_at"`
	Value     json.RawMessage `json:"value"`
}

func (s *Storage) CacheGet(namespace, key string, destination any) (bool, error) {
	if s.cache == nil {
		return false, errors.New("cache storage is not open")
	}
	var raw []byte
	err := s.cache.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(cacheBucket(namespace))
		if bucket != nil {
			raw = bytes.Clone(bucket.Get([]byte(key)))
		}
		return nil
	})
	if err != nil || raw == nil {
		return false, err
	}

	var entry cacheEntry
	if json.Unmarshal(raw, &entry) != nil || entry.ExpiresAt <= s.now().UnixNano() || json.Unmarshal(entry.Value, destination) != nil {
		_ = s.deleteCacheEntry(namespace, key, raw, entry.ExpiresAt)
		return false, nil
	}
	return true, nil
}

func (s *Storage) CachePut(namespace, key string, value any, ttl time.Duration) error {
	if s.cache == nil {
		return errors.New("cache storage is not open")
	}
	if ttl <= 0 {
		return errors.New("cache TTL must be positive")
	}
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	expiresAt := s.now().Add(ttl).UnixNano()
	raw, err := json.Marshal(cacheEntry{ExpiresAt: expiresAt, Value: payload})
	if err != nil {
		return err
	}

	return s.cache.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(cacheBucket(namespace))
		if err != nil {
			return err
		}
		expiry, err := tx.CreateBucketIfNotExists(expiryBucket)
		if err != nil {
			return err
		}
		if previous := bucket.Get([]byte(key)); previous != nil {
			var entry cacheEntry
			if json.Unmarshal(previous, &entry) == nil {
				if err := expiry.Delete(expiryKey(entry.ExpiresAt, namespace, key)); err != nil {
					return err
				}
			}
		}
		if err := bucket.Put([]byte(key), raw); err != nil {
			return err
		}
		if err := expiry.Put(expiryKey(expiresAt, namespace, key), expiryValue(namespace, key)); err != nil {
			return err
		}
		return sweepExpired(tx, s.now().UnixNano())
	})
}

func (s *Storage) deleteCacheEntry(namespace, key string, expected []byte, expiresAt int64) error {
	return s.cache.Update(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(cacheBucket(namespace))
		if bucket == nil || !bytes.Equal(bucket.Get([]byte(key)), expected) {
			return nil
		}
		if err := bucket.Delete([]byte(key)); err != nil {
			return err
		}
		if expiry := tx.Bucket(expiryBucket); expiry != nil && expiresAt != 0 {
			return expiry.Delete(expiryKey(expiresAt, namespace, key))
		}
		return nil
	})
}

func sweepExpired(tx *bolt.Tx, now int64) error {
	expiry := tx.Bucket(expiryBucket)
	if expiry == nil {
		return nil
	}
	cursor := expiry.Cursor()
	count := 0
	for key, value := cursor.First(); key != nil && count < expirySweepLimit; key, value = cursor.Next() {
		if len(key) < 8 || int64(binary.BigEndian.Uint64(key[:8])) > now {
			break
		}
		namespace, primaryKey, ok := bytes.Cut(value, []byte{0})
		if ok {
			if bucket := tx.Bucket(cacheBucket(string(namespace))); bucket != nil {
				var entry cacheEntry
				if json.Unmarshal(bucket.Get(primaryKey), &entry) != nil || entry.ExpiresAt <= now {
					if err := bucket.Delete(primaryKey); err != nil {
						return err
					}
				}
			}
		}
		if err := cursor.Delete(); err != nil {
			return err
		}
		count++
	}
	return nil
}

func cacheBucket(namespace string) []byte {
	return []byte("cache:" + namespace)
}

func expiryKey(expiresAt int64, namespace, key string) []byte {
	result := make([]byte, 8+sha256.Size)
	binary.BigEndian.PutUint64(result, uint64(expiresAt))
	sum := sha256.Sum256(expiryValue(namespace, key))
	copy(result[8:], sum[:])
	return result
}

func expiryValue(namespace, key string) []byte {
	result := make([]byte, 0, len(namespace)+len(key)+1)
	result = append(result, namespace...)
	result = append(result, 0)
	return append(result, key...)
}
