package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

type HistoryEntry struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	Type     string    `json:"type"`
	PlayedAt time.Time `json:"played_at"`
}

func History() ([]HistoryEntry, error) {
	data, err := os.ReadFile(historyPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var entries []HistoryEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, err
	}
	return entries, nil
}

func RecordHistory(entry HistoryEntry) error {
	entries, err := History()
	if err != nil {
		return err
	}
	return recordHistory(entries, entry)
}

func ToggleHistory(entry HistoryEntry) (bool, error) {
	entries, err := History()
	if err != nil {
		return false, err
	}
	for _, existing := range entries {
		if existing.ID == entry.ID {
			return false, removeHistory(entries, entry.ID)
		}
	}
	return true, recordHistory(entries, entry)
}

func RemoveHistory(id string) error {
	entries, err := History()
	if err != nil {
		return err
	}
	return removeHistory(entries, id)
}

func recordHistory(entries []HistoryEntry, entry HistoryEntry) error {
	if entry.PlayedAt.IsZero() {
		entry.PlayedAt = time.Now().UTC()
	}
	updated := []HistoryEntry{entry}
	for _, existing := range entries {
		if existing.ID != entry.ID {
			updated = append(updated, existing)
		}
		if len(updated) == 100 {
			break
		}
	}
	return writeJSON(historyPath(), updated)
}

func removeHistory(entries []HistoryEntry, id string) error {
	updated := make([]HistoryEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ID != id {
			updated = append(updated, entry)
		}
	}
	return writeJSON(historyPath(), updated)
}

func historyPath() string {
	return filepath.Join(configRoot(), "lemmewatch", "history.json")
}

func writeJSON(filename string, value any) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(filename), "lemmewatch-*.json")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(temporaryName, filename)
}

func configRoot() string {
	root, err := os.UserConfigDir()
	if err != nil || root == "" {
		return "."
	}
	return root
}
