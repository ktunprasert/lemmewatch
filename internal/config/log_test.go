package config

import (
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLogFailureUsesXDGStateAndRedactsURLs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("XDG_STATE_HOME", root)
	secretURL := "https://example.invalid/video?token=secret"
	if err := LogFailure("player", errors.New("failed "+secretURL)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(LogPath())
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "player: failed [redacted-url]") || strings.Contains(text, secretURL) {
		t.Fatalf("log = %q", text)
	}
	info, err := os.Stat(LogPath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o", info.Mode().Perm())
	}
}
