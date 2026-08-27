package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesRoundTrip(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", root)
	if err := Save(Preferences{Quality: 1080, MediaTab: "series"}); err != nil {
		t.Fatal(err)
	}
	if got := Load().Quality; got != 1080 {
		t.Fatalf("quality = %d", got)
	}
	if got := Load().MediaTab; got != "series" {
		t.Fatalf("media tab = %q", got)
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
	if got := Load(); got != (Preferences{}) {
		t.Fatalf("preferences = %#v", got)
	}
}
