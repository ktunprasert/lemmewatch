package config

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"lemmewatch/internal/model"
)

func TestPreferencesRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	cachedOnly := false
	rememberPlayback := false
	if err := Save(Preferences{PlaybackPreferences: model.PlaybackPreferences{AudioLanguages: []string{"ja", "en"}, SubtitleLanguages: []string{"en", "fr"}, PlaybackSpeed: 1.25}, Quality: 1080, MediaTab: "series", CachedOnly: &cachedOnly, Provider: "webstreamr", TorBoxToken: "secret", Player: "vlc", Autoplay: true, RememberPlayback: &rememberPlayback, DetailModes: map[string]string{"media": "i"}, PaneSizes: map[int][]int{2: {1, 1}, 3: {1, 2, 3}}}); err != nil {
		t.Fatal(err)
	}
	if got := Load().Quality; got != 1080 {
		t.Fatalf("quality = %d", got)
	}
	if got := Load(); !slices.Equal(got.AudioLanguages, []string{"ja", "en"}) || !slices.Equal(got.SubtitleLanguages, []string{"en", "fr"}) || got.PlaybackSpeed != 1.25 {
		t.Fatalf("playback preferences = %#v", got.PlaybackPreferences)
	}
	if got := Load().MediaTab; got != "series" {
		t.Fatalf("media tab = %q", got)
	}
	if got := Load().DetailModes["media"]; got != "i" {
		t.Fatalf("media detail mode = %q", got)
	}
	if got := Load().PaneSizes; !slices.Equal(got[2], []int{1, 1}) || !slices.Equal(got[3], []int{1, 2, 3}) {
		t.Fatalf("pane sizes = %v", got)
	}
	if got := Load(); got.CachedOnly == nil || *got.CachedOnly || got.Provider != "webstreamr" || got.TorBoxToken != "secret" || got.Player != "vlc" || !got.Autoplay || got.RememberPlaybackEnabled() {
		t.Fatalf("saved defaults = %#v", got)
	}
	info, err := os.Stat(filepath.Join(root, "lemmewatch", "preferences.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}

func TestLoadIgnoresInvalidPreferences(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	directory := filepath.Join(root, "lemmewatch")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "preferences.json"), []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := Load(); got.Quality != 0 || got.MediaTab != "" || len(got.DetailModes) != 0 {
		t.Fatalf("preferences = %#v", got)
	}
	if !Load().RememberPlaybackEnabled() {
		t.Fatal("missing preference disabled playback memory")
	}
}
