package player

import (
	"bytes"
	"context"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"lemmewatch/internal/model"
)

func TestPlayerPreferenceArguments(t *testing.T) {
	prefs := model.PlaybackPreferences{AudioLanguages: []string{"jpn", "en"}, SubtitleLanguages: []string{"pt-BR", "eng"}, PlaybackSpeed: 1.25}
	for _, tc := range []struct {
		executable string
		want       []string
	}{
		{`C:\Program Files\mpv\mpv.exe`, []string{"--alang=ja,en", "--aid=auto", "--slang=pt-BR,en", "--sid=auto", "--sub-visibility=yes", "--subs-with-matching-audio=yes", "--speed=1.25"}},
		{"/usr/bin/vlc", []string{"--audio-language=ja,en,any", "--audio-track=-1", "--audio-track-id=-1", "--audio", "--sub-language=pt,en,any", "--sub-track=-1", "--sub-track-id=-1", "--spu", "--rate=1.25"}},
		{"xdg-open", nil}, {"open", nil}, {"rundll32", nil}, {"custom-player", nil},
	} {
		got := (Player{Executable: tc.executable, Preferences: prefs}).preferenceArguments()
		if !slices.Equal(got, tc.want) {
			t.Errorf("%s: %v", tc.executable, got)
		}
	}
	for _, speed := range []float64{0, -1, 5, math.NaN(), math.Inf(1)} {
		got := (Player{Executable: "mpv", Preferences: model.PlaybackPreferences{AudioLanguages: []string{"--bad"}, PlaybackSpeed: speed}}).preferenceArguments()
		if len(got) != 0 {
			t.Errorf("invalid preferences generated flags: %v", got)
		}
	}
}

func TestPlayPassesPreferencesAfterCustomArguments(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell fixture")
	}
	executable := filepath.Join(t.TempDir(), "mpv")
	if err := os.WriteFile(executable, []byte("#!/bin/sh\nprintf '%s\\n' \"$@\"\n"), 0o700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	p := Player{Executable: executable, Arguments: []string{"--alang=fr", "--speed=2"}, Stdout: &output,
		Preferences: model.PlaybackPreferences{AudioLanguages: []string{"ja", "en"}, PlaybackSpeed: 1.5}}
	if err := p.Play(context.Background(), model.Playback{URL: "https://example.invalid/video"}); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(output.String()); got != "--alang=fr\n--speed=2\n--alang=ja,en\n--aid=auto\n--speed=1.5\nhttps://example.invalid/video" {
		t.Fatalf("arguments: %q", got)
	}
}
