package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	cachedOnly := false
	if err := Save(Preferences{Quality: 1080, MediaTab: "series", CachedOnly: &cachedOnly, Player: "vlc", DetailModes: map[string]string{"media": "i"}}); err != nil {
		t.Fatal(err)
	}
	if got := Load().Quality; got != 1080 {
		t.Fatalf("quality = %d", got)
	}
	if got := Load().MediaTab; got != "series" {
		t.Fatalf("media tab = %q", got)
	}
	if got := Load().DetailModes["media"]; got != "i" {
		t.Fatalf("media detail mode = %q", got)
	}
	if got := Load(); got.CachedOnly == nil || *got.CachedOnly || got.Player != "vlc" {
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
}
