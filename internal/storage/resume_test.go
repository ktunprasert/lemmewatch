package storage

import "testing"

func TestResumePositionsPersistIndependentlyOfWatchedState(t *testing.T) {
	store := openTestStorage(t)
	entry := HistoryEntry{ID: "tt1", Title: "Show", Type: "series", Episodes: []string{"1:1"}}
	if err := store.RecordHistory(entry); err != nil {
		t.Fatal(err)
	}
	positions := map[string]int{"series:tt1:1:1": 125, "series:tt1:1:2": 450, "movie:tt2": 900}
	for key, seconds := range positions {
		if err := store.SaveResumePosition(key, seconds); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	for key, want := range positions {
		if got, err := store.ResumePosition(key); err != nil || got != want {
			t.Fatalf("ResumePosition(%q) = %d, %v; want %d", key, got, err, want)
		}
	}
	if got, err := store.ResumePosition("series:tt1:2:1"); err != nil || got != 0 {
		t.Fatalf("unplayed episode = %d, %v", got, err)
	}
	if err := store.SaveResumePosition("series:tt1:1:1", 0); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ResumePosition("series:tt1:1:1"); err != nil || got != 0 {
		t.Fatalf("cleared checkpoint = %d, %v", got, err)
	}
	state, err := store.Watched()
	if err != nil || !state["tt1:1:1"] || state["tt1:1:2"] || state["tt2"] {
		t.Fatalf("resume changed watched state: %#v, %v", state, err)
	}
	if _, err := store.ToggleWatched(entry, []string{"1:2"}); err != nil {
		t.Fatal(err)
	}
	if got, err := store.ResumePosition("series:tt1:1:2"); err != nil || got != 450 {
		t.Fatalf("watched toggle changed checkpoint: %d, %v", got, err)
	}
}

func TestResumeStorageRejectsInvalidAndClosedWrites(t *testing.T) {
	store := openTestStorage(t)
	if err := store.SaveResumePosition("", 10); err == nil {
		t.Fatal("empty key accepted")
	}
	if err := store.SaveResumePosition("movie:tt1", -10); err == nil {
		t.Fatal("negative position accepted")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.ResumePosition("movie:tt1"); err == nil {
		t.Fatal("closed read accepted")
	}
	if err := store.SaveResumePosition("movie:tt1", 10); err == nil {
		t.Fatal("closed write accepted")
	}
}
