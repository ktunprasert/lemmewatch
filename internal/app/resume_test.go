package app

import (
	"math"
	"path/filepath"
	"testing"

	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/storage"
)

func TestResumePlayerTracksMediaAcrossStreamsAndPlayers(t *testing.T) {
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	selected := navigationChoice{
		media:   model.Media{ID: "tt1", Type: model.Series},
		episode: model.Episode{ID: "tt1:1:2", Season: 1, Episode: 2},
		stream:  model.Stream{URL: "https://example.invalid/old?token=secret"},
	}
	a := App{Storage: store, Player: player.Player{Executable: "vlc"}}
	p := a.resumePlayer(selected)
	if p.ResumeSeconds != 0 || p.OnProgress == nil {
		t.Fatalf("new playback = %#v", p)
	}
	p.OnProgress(player.Progress{Position: 123.8, Duration: 1800})
	a.Player.Executable = `C:\Players\mpv.exe`
	selected.stream.URL = "https://example.invalid/refreshed?token=different"
	p = a.resumePlayer(selected)
	if p.ResumeSeconds != 123 {
		t.Fatalf("resume = %d; want saved second", p.ResumeSeconds)
	}
	for _, position := range []float64{0, 0.5, -1, math.NaN(), math.Inf(1)} {
		p.OnProgress(player.Progress{Position: position})
	}
	if got := a.resumePlayer(selected).ResumeSeconds; got != 123 {
		t.Fatalf("startup/invalid position erased checkpoint: %d", got)
	}
	otherEpisode := selected
	otherEpisode.episode = model.Episode{ID: "tt1:1:3", Season: 1, Episode: 3}
	if got := a.resumePlayer(otherEpisode).ResumeSeconds; got != 0 {
		t.Fatalf("checkpoint leaked into another episode: %d", got)
	}
	p.OnProgress(player.Progress{Position: 40, Duration: 1800})
	if got := a.resumePlayer(selected).ResumeSeconds; got != 40 {
		t.Fatalf("backward seek not saved: %d", got)
	}
	p.OnProgress(player.Progress{Position: 1787, Duration: 1800})
	if got := a.resumePlayer(selected).ResumeSeconds; got != 0 {
		t.Fatalf("completed media still resumes: %d", got)
	}
	a.Player.Executable = "xdg-open"
	if p := a.resumePlayer(selected); p.OnProgress != nil || p.ResumeSeconds != 0 {
		t.Fatalf("URL handler enabled tracking: %#v", p)
	}
	if a.Player.OnProgress != nil {
		t.Fatal("session tracking changed configured player")
	}
}

func TestResumeKeySeparatesMoviesSeasonsAndEpisodes(t *testing.T) {
	for _, test := range []struct {
		choice navigationChoice
		want   string
	}{
		{navigationChoice{}, ""},
		{navigationChoice{media: model.Media{ID: "tt1", Type: model.Movie}}, "movie:tt1"},
		{navigationChoice{media: model.Media{ID: "tt1", Type: model.Series}, episode: model.Episode{ID: "special", Season: 0, Episode: 1}}, "series:tt1:0:1"},
		{navigationChoice{media: model.Media{ID: "tt1", Type: model.Series}, episode: model.Episode{ID: "episode", Season: 2, Episode: 1}}, "series:tt1:2:1"},
	} {
		if got := test.choice.resumeKey(); got != test.want {
			t.Fatalf("key = %q, want %q", got, test.want)
		}
	}
}
