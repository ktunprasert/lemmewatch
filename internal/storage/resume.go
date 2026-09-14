package storage

import (
	"errors"
	"strconv"

	bolt "go.etcd.io/bbolt"
)

var resumeBucket = []byte("resume")

func (s *Storage) ResumePosition(key string) (int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.history == nil {
		return 0, errors.New("history storage is not open")
	}
	var seconds int
	err := s.history.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(resumeBucket)
		if bucket == nil || bucket.Get([]byte(key)) == nil {
			return nil
		}
		var err error
		seconds, err = strconv.Atoi(string(bucket.Get([]byte(key))))
		return err
	})
	return seconds, err
}

// SaveResumePosition stores a checkpoint independently of history and watched state.
// A zero position clears the checkpoint.
func (s *Storage) SaveResumePosition(key string, seconds int) error {
	if key == "" || seconds < 0 {
		return errors.New("invalid resume checkpoint")
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.history == nil {
		return errors.New("history storage is not open")
	}
	return s.history.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(resumeBucket)
		if err != nil {
			return err
		}
		if seconds == 0 {
			return bucket.Delete([]byte(key))
		}
		return bucket.Put([]byte(key), []byte(strconv.Itoa(seconds)))
	})
}
