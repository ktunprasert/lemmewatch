package config

import (
	"testing"
	"time"
)

func TestHistoryRecordsNewestUniqueTitles(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	first := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	if err := RecordHistory(HistoryEntry{ID: "tt1", Title: "First", Type: "movie", PlayedAt: first}); err != nil {
		t.Fatal(err)
	}
	if err := RecordHistory(HistoryEntry{ID: "tt2", Title: "Second", Type: "series", PlayedAt: second}); err != nil {
		t.Fatal(err)
	}
	if err := RecordHistory(HistoryEntry{ID: "tt1", Title: "First Again", Type: "movie", PlayedAt: second}); err != nil {
		t.Fatal(err)
	}
	entries, err := History()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].ID != "tt1" || entries[0].Title != "First Again" || entries[1].ID != "tt2" {
		t.Fatalf("history = %#v", entries)
	}
}

func TestHistoryMissingFileIsEmpty(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	entries, err := History()
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("history = %#v", entries)
	}
}
