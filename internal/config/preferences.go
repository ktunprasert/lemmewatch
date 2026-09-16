package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"lemmewatch/internal/model"
)

type Preferences struct {
	model.PlaybackPreferences
	Quality     int               `json:"quality,omitempty"`
	MediaTab    string            `json:"media_tab,omitempty"`
	CachedOnly  *bool             `json:"cached_only,omitempty"`
	Provider    string            `json:"provider,omitempty"`
	TorBoxToken string            `json:"torbox_api_token,omitempty"`
	Player      string            `json:"player,omitempty"`
	Autoplay    bool              `json:"autoplay,omitempty"`
	DetailModes map[string]string `json:"detail_modes,omitempty"`
	PaneSizes   map[int][]int     `json:"pane_sizes,omitempty"`
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
