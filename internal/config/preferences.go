package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Preferences struct {
	Quality     int               `json:"quality,omitempty"`
	MediaTab    string            `json:"media_tab,omitempty"`
	DetailModes map[string]string `json:"detail_modes,omitempty"`
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
	return writeJSON(path(), preferences)
}

func path() string {
	return filepath.Join(configRoot(), "lemmewatch", "preferences.json")
}
