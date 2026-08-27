package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Preferences struct {
	Quality int `json:"quality,omitempty"`
}

func Load() Preferences {
	data, err := os.ReadFile(path())
	if err != nil {
		return Preferences{}
	}
	var preferences Preferences
	if json.Unmarshal(data, &preferences) != nil {
		return Preferences{}
	}
	return preferences
}

func Save(preferences Preferences) error {
	filename := path()
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(preferences, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(filepath.Dir(filename), "preferences-*.json")
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

func path() string {
	root, err := os.UserConfigDir()
	if err != nil || root == "" {
		root = "."
	}
	return filepath.Join(root, "lemmewatch", "preferences.json")
}
