package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/model"
	"lemmewatch/internal/storage"
)

func TestInfoUsesFullCachedMetadataAndRefreshesEpisodes(t *testing.T) {
	requests, cast, overview := 0, "First actor", "Original overview"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"meta":{"id":"tt1","type":"series","name":"Full show title","imdbRating":"7.0","cast":["` + cast + `"],"status":"Returning Series","videos":[{"id":"tt1:1:1","season":1,"episode":1,"name":"Pilot","overview":"` + overview + `"}]}}`))
	}))
	defer server.Close()
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	a := App{Catalog: catalog.Client{BaseURL: server.URL, HTTP: server.Client()}, Storage: store}
	selected := navigationChoice{kind: navigationMedia, media: model.Media{ID: "tt1", Type: model.Series, Name: "Search title"}}
	full, err := a.navigationInfo(context.Background(), selected)
	if err != nil || !strings.Contains(strings.Join(full.InfoLines(nil), "\n"), "First actor") {
		t.Fatalf("info = %#v, %v", full, err)
	}
	if full.media.Name != selected.media.Name {
		t.Fatal("metadata changed the selected title")
	}
	for _, mode := range selected.WithInfo(full).ContextModes() {
		if mode.Key == "r" && mode.Value != "7.0" {
			t.Fatalf("Cinemeta rating missing from row mode: %#v", mode)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	selected.kind, selected.episode = navigationEpisode, model.Episode{Season: 1, Episode: 1}
	full, err = a.navigationInfo(context.Background(), selected)
	if err != nil || requests != 1 || !strings.Contains(strings.Join(full.InfoLines(nil), "\n"), "Original overview") {
		t.Fatalf("cached info = %#v, %v requests=%d", full, err, requests)
	}
	cast, overview = "New actor", "Updated overview"
	if _, err := a.seriesEpisodes(context.Background(), "tt1", true); err != nil {
		t.Fatal(err)
	}
	full, err = a.navigationInfo(context.Background(), selected)
	if err != nil || requests != 2 || !strings.Contains(strings.Join(full.InfoLines(nil), "\n"), "Updated overview") {
		t.Fatal("refresh retained old info")
	}
}

func TestRatingModeUsesTitleRatingAndLabelsEpisodeFallback(t *testing.T) {
	for _, tc := range []struct {
		choice navigationChoice
		want   string
		needs  bool
	}{
		{navigationChoice{kind: navigationMedia, media: model.Media{Rating: "7.0"}}, "7.0", false},
		{navigationChoice{kind: navigationMedia}, "--", true},
		{navigationChoice{kind: navigationEpisode, media: model.Media{Rating: "7.0"}}, "7.0 show", false},
		{navigationChoice{kind: navigationEpisode, media: model.Media{Rating: "7.0"}, episode: model.Episode{Rating: "8.4"}}, "8.4", false},
		{navigationChoice{kind: navigationEpisode}, "--", true},
	} {
		for _, mode := range tc.choice.ContextModes() {
			if mode.Key == "r" && mode.Value != tc.want {
				t.Fatalf("rating mode = %q, want %q", mode.Value, tc.want)
			}
		}
		if tc.choice.ModeNeedsInfo("r") != tc.needs || tc.choice.ModeNeedsInfo("i") {
			t.Fatal("wrong metadata load requirements")
		}
	}
}

func TestInfoIdentityAndLocalStateStayCurrent(t *testing.T) {
	first := navigationChoice{kind: navigationStream, media: model.Media{ID: "tt1"}, stream: model.Stream{Provider: "torbox", Hash: strings.Repeat("a", 40), Filename: "S01E01.mkv"}, episode: model.Episode{Season: 1, Episode: 1}}
	second := first
	second.stream.Filename, second.episode.Episode = "S01E02.mkv", 2
	if first.InfoKey() == second.InfoKey() {
		t.Fatal("season pack episode info identities collide")
	}
	current := navigationChoice{kind: navigationMedia, media: model.Media{ID: "tt1", PlayedAt: time.Now()}, playedAt: time.Now()}
	oldInfo := current
	oldInfo.media.PlayedAt, oldInfo.playedAt = time.Time{}, time.Time{}
	oldInfo.media.Summary = "Full summary"
	merged := current.WithInfo(oldInfo)
	if merged.media.PlayedAt != current.media.PlayedAt || merged.playedAt != current.playedAt || merged.media.Summary != "Full summary" {
		t.Fatal("info snapshot overwrote local state")
	}
}
