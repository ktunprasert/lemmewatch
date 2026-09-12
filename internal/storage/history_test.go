package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTestStorage(t *testing.T) *Storage {
	t.Helper()
	root := t.TempDir()
	storage := NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := storage.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	return storage
}

func TestHistoryRecordsNewestUniqueTitles(t *testing.T) {
	storage := openTestStorage(t)
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	if err := storage.RecordHistory(HistoryEntry{ID: "tt1", Title: "First", Type: "movie", PlayedAt: first}); err != nil {
		t.Fatal(err)
	}
	if err := storage.RecordHistory(HistoryEntry{ID: "tt2", Title: "Second", Type: "series", PlayedAt: second}); err != nil {
		t.Fatal(err)
	}
	if err := storage.RecordHistory(HistoryEntry{ID: "tt1", Title: "First Again", Type: "movie", PlayedAt: second}); err != nil {
		t.Fatal(err)
	}
	entries, err := storage.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != "tt1" || entries[0].Title != "First Again" || entries[1].ID != "tt2" {
		t.Fatalf("history = %#v", entries)
	}
}

func TestHistoryStartsEmpty(t *testing.T) {
	storage := openTestStorage(t)
	entries, err := storage.History()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("history = %#v", entries)
	}
}

func TestHistoryCanToggleAndRemoveTitles(t *testing.T) {
	storage := openTestStorage(t)
	entry := HistoryEntry{ID: "tt1", Title: "Dune", Type: "movie"}
	added, err := storage.ToggleHistory(entry)
	if err != nil || !added {
		t.Fatalf("add toggle = %t, %v", added, err)
	}
	added, err = storage.ToggleHistory(entry)
	if err != nil || added {
		t.Fatalf("remove toggle = %t, %v", added, err)
	}
	if err := storage.RecordHistory(entry); err != nil {
		t.Fatal(err)
	}
	if err := storage.RemoveHistory(entry.ID); err != nil {
		t.Fatal(err)
	}
	entries, err := storage.History()
	if err != nil || len(entries) != 0 {
		t.Fatalf("history = %#v, %v", entries, err)
	}
}

func TestWatchedTogglesEpisodeSetsAndReadsLegacyEntries(t *testing.T) {
	storage := openTestStorage(t)
	entry := HistoryEntry{ID: "tt1", Title: "Silo", Type: "series"}
	state, err := storage.ToggleWatched(entry, []string{"1:2", "1:1"})
	if err != nil || !state["tt1"] || !state["tt1:1:1"] || !state["tt1:1:2"] {
		t.Fatalf("watched state = %#v, %v", state, err)
	}
	state, err = storage.ToggleWatched(entry, []string{"1:1", "1:2"})
	if err != nil || state["tt1:1:1"] || state["tt1:1:2"] || !state["tt1"] {
		t.Fatalf("cleared season = %#v, %v", state, err)
	}
	entries, err := storage.History()
	if err != nil || len(entries) != 1 || entries[0].Title != "Silo" {
		t.Fatalf("compatible history = %#v, %v", entries, err)
	}
}

func TestLegacyHistoryMigratesOnce(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "history.json")
	if err := os.WriteFile(legacy, []byte(`[{"id":"tt1","title":"Dune","type":"movie","played_at":"2026-01-01T00:00:00Z"}]`), 0o600); err != nil {
		t.Fatal(err)
	}
	storage := NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), legacy)
	if err := storage.Open(); err != nil {
		t.Fatal(err)
	}
	entries, err := storage.History()
	if err != nil || len(entries) != 1 || entries[0].ID != "tt1" {
		t.Fatalf("history = %#v, %v", entries, err)
	}
	if _, err := os.Stat(legacy + ".migrated"); err != nil {
		t.Fatalf("migration backup: %v", err)
	}
	if err := storage.RemoveHistory("tt1"); err != nil {
		t.Fatal(err)
	}
	if err := storage.Close(); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(legacy+".migrated", legacy); err != nil {
		t.Fatal(err)
	}
	if err := storage.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	entries, err = storage.History()
	if err != nil || len(entries) != 0 {
		t.Fatalf("history was reimported: %#v, %v", entries, err)
	}
}

func TestMalformedLegacyHistoryBlocksMigration(t *testing.T) {
	root := t.TempDir()
	legacy := filepath.Join(root, "history.json")
	if err := os.WriteFile(legacy, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	storage := NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), legacy)
	if err := storage.Open(); err == nil {
		storage.Close()
		t.Fatal("malformed history migrated")
	}
	if _, err := os.Stat(legacy); err != nil {
		t.Fatalf("legacy history removed: %v", err)
	}
}

func TestSecondSessionTimesOut(t *testing.T) {
	root := t.TempDir()
	paths := []string{filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json")}
	first := NewAt(paths[0], paths[1], paths[2])
	if err := first.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = first.Close() })
	second := NewAt(paths[0], filepath.Join(root, "second-cache.db"), paths[2])
	started := time.Now()
	err := second.Open()
	if err != ErrSessionActive {
		t.Fatalf("second open = %v", err)
	}
	if elapsed := time.Since(started); elapsed < 200*time.Millisecond || elapsed > time.Second {
		t.Fatalf("lock timeout = %v", elapsed)
	}
}
