package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	historyBucket    = []byte("history")
	historyEntries   = []byte("entries")
	metadataBucket   = []byte("metadata")
	historyMigration = []byte("history-json-v1")
)

type HistoryEntry struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Type     string    `json:"type"`
	PlayedAt time.Time `json:"played_at"`
	Episodes []string  `json:"episodes,omitempty"`
}

type WatchedState map[string]bool

func (s *Storage) migrateLegacyHistory() error {
	legacy, readErr := os.ReadFile(s.legacyPath)
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}
	if readErr == nil {
		var entries []HistoryEntry
		if err := json.Unmarshal(legacy, &entries); err != nil {
			return err
		}
	}

	imported := false
	err := s.history.Update(func(tx *bolt.Tx) error {
		metadata, err := tx.CreateBucketIfNotExists(metadataBucket)
		if err != nil {
			return err
		}
		if metadata.Get(historyMigration) != nil {
			return nil
		}
		history, err := tx.CreateBucketIfNotExists(historyBucket)
		if err != nil {
			return err
		}
		if readErr == nil && history.Get(historyEntries) == nil {
			if err := history.Put(historyEntries, legacy); err != nil {
				return err
			}
			imported = true
		}
		return metadata.Put(historyMigration, []byte{1})
	})
	if err != nil || !imported {
		return err
	}
	if err := os.Rename(s.legacyPath, s.legacyPath+".migrated"); err != nil {
		return fmt.Errorf("archive legacy history: %w", err)
	}
	return nil
}

func (s *Storage) History() ([]HistoryEntry, error) {
	if s.history == nil {
		return nil, errors.New("history storage is not open")
	}
	var entries []HistoryEntry
	err := s.history.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket(historyBucket)
		if bucket == nil || bucket.Get(historyEntries) == nil {
			return nil
		}
		return json.Unmarshal(bucket.Get(historyEntries), &entries)
	})
	return entries, err
}

func (s *Storage) RecordHistory(entry HistoryEntry) error {
	_, err := s.updateHistory(func(entries []HistoryEntry) ([]HistoryEntry, error) {
		return recordHistory(entries, entry), nil
	})
	return err
}

func (s *Storage) ToggleHistory(entry HistoryEntry) (bool, error) {
	added := true
	_, err := s.updateHistory(func(entries []HistoryEntry) ([]HistoryEntry, error) {
		for _, existing := range entries {
			if existing.ID == entry.ID {
				added = false
				return removeHistory(entries, entry.ID), nil
			}
		}
		return recordHistory(entries, entry), nil
	})
	return added, err
}

func (s *Storage) Watched() (WatchedState, error) {
	entries, err := s.History()
	if err != nil {
		return nil, err
	}
	return watched(entries), nil
}

func (s *Storage) ToggleWatched(entry HistoryEntry, episodeKeys []string) (WatchedState, error) {
	if len(episodeKeys) == 0 {
		if _, err := s.ToggleHistory(entry); err != nil {
			return nil, err
		}
		return s.Watched()
	}

	entries, err := s.updateHistory(func(entries []HistoryEntry) ([]HistoryEntry, error) {
		index := -1
		for i := range entries {
			if entries[i].ID == entry.ID {
				index = i
				break
			}
		}
		if index < 0 {
			entries = append([]HistoryEntry{entry}, entries...)
			index = 0
		}
		existing := make(map[string]bool, len(entries[index].Episodes))
		for _, key := range entries[index].Episodes {
			existing[key] = true
		}
		allWatched := true
		for _, key := range episodeKeys {
			allWatched = allWatched && existing[key]
		}
		for _, key := range episodeKeys {
			if allWatched {
				delete(existing, key)
			} else {
				existing[key] = true
			}
		}
		entries[index].Episodes = entries[index].Episodes[:0]
		for key := range existing {
			entries[index].Episodes = append(entries[index].Episodes, key)
		}
		sort.Strings(entries[index].Episodes)
		return entries, nil
	})
	if err != nil {
		return nil, err
	}
	return watched(entries), nil
}

func (s *Storage) RemoveHistory(id string) error {
	_, err := s.updateHistory(func(entries []HistoryEntry) ([]HistoryEntry, error) {
		return removeHistory(entries, id), nil
	})
	return err
}

func (s *Storage) updateHistory(update func([]HistoryEntry) ([]HistoryEntry, error)) ([]HistoryEntry, error) {
	if s.history == nil {
		return nil, errors.New("history storage is not open")
	}
	var updated []HistoryEntry
	err := s.history.Update(func(tx *bolt.Tx) error {
		bucket, err := tx.CreateBucketIfNotExists(historyBucket)
		if err != nil {
			return err
		}
		var entries []HistoryEntry
		if data := bucket.Get(historyEntries); data != nil {
			if err := json.Unmarshal(data, &entries); err != nil {
				return err
			}
		}
		updated, err = update(entries)
		if err != nil {
			return err
		}
		data, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		return bucket.Put(historyEntries, data)
	})
	return updated, err
}

func recordHistory(entries []HistoryEntry, entry HistoryEntry) []HistoryEntry {
	if entry.PlayedAt.IsZero() {
		entry.PlayedAt = time.Now().UTC()
	}
	updated := []HistoryEntry{entry}
	for _, existing := range entries {
		if existing.ID == entry.ID {
			episodes := make(map[string]bool, len(existing.Episodes)+len(entry.Episodes))
			for _, key := range existing.Episodes {
				episodes[key] = true
			}
			for _, key := range entry.Episodes {
				episodes[key] = true
			}
			updated[0].Episodes = updated[0].Episodes[:0]
			for key := range episodes {
				updated[0].Episodes = append(updated[0].Episodes, key)
			}
			sort.Strings(updated[0].Episodes)
		}
		if existing.ID != entry.ID {
			updated = append(updated, existing)
		}
		if len(updated) == 100 {
			break
		}
	}
	return updated
}

func removeHistory(entries []HistoryEntry, id string) []HistoryEntry {
	updated := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ID != id {
			updated = append(updated, entry)
		}
	}
	return updated
}

func watched(entries []HistoryEntry) WatchedState {
	state := make(WatchedState)
	for _, entry := range entries {
		if entry.Title != "" {
			state[entry.ID] = true
		}
		for _, episode := range entry.Episodes {
			state[entry.ID+":"+episode] = true
		}
	}
	return state
}
