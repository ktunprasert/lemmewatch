package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/provider"
	"lemmewatch/internal/storage"
	"lemmewatch/internal/torbox"
)

func TestSearchPreservesCatalogRelevanceWithinMediaType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/movie/"):
			_, _ = w.Write([]byte(`{"metas":[{"id":"tt2","type":"movie","name":"Zulu"},{"id":"tt1","type":"movie","name":"Alpha"}]}`))
		case strings.Contains(r.URL.Path, "/series/"):
			_, _ = w.Write([]byte(`{"metas":[{"id":"tt4","type":"series","name":"Yellow"},{"id":"tt3","type":"series","name":"Beta"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	a := App{Catalog: catalog.Client{BaseURL: server.URL, HTTP: server.Client()}, Err: io.Discard}
	items, err := a.Search(context.Background(), "query", "")
	if err != nil {
		t.Fatal(err)
	}
	var movies, series []string
	for _, item := range items {
		switch item.Type {
		case model.Movie:
			movies = append(movies, item.Name)
		case model.Series:
			series = append(series, item.Name)
		}
	}
	if strings.Join(movies, ",") != "Zulu,Alpha" {
		t.Fatalf("movie order = %#v", movies)
	}
	if strings.Join(series, ",") != "Yellow,Beta" {
		t.Fatalf("series order = %#v", series)
	}
}

func TestSearchUsesPersistentCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"metas":[{"id":"tt1","type":"movie","name":"Dune"}]}`))
	}))
	defer server.Close()
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a := App{Catalog: catalog.Client{BaseURL: server.URL, HTTP: server.Client()}, Storage: store, Err: io.Discard}
	for range 2 {
		items, err := a.Search(context.Background(), "Dune", model.Movie)
		if err != nil || len(items) != 1 {
			t.Fatalf("search = %#v, %v", items, err)
		}
	}
	if requests != 1 {
		t.Fatalf("catalog requests = %d", requests)
	}
}

func TestSeriesEpisodesUsePersistentCache(t *testing.T) {
	requests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		_, _ = w.Write([]byte(`{"meta":{"videos":[{"id":"tt1:1:1","name":"Pilot","season":1,"episode":1}]}}`))
	}))
	defer server.Close()
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a := App{Catalog: catalog.Client{BaseURL: server.URL, HTTP: server.Client()}, Storage: store}
	for range 2 {
		episodes, err := a.seriesEpisodes(context.Background(), "tt1", false)
		if err != nil || len(episodes) != 1 {
			t.Fatalf("episodes = %#v, %v", episodes, err)
		}
	}
	if requests != 1 {
		t.Fatalf("catalog requests = %d", requests)
	}
}

func TestHistoryMediaPreservesPlayableEntries(t *testing.T) {
	playedAt := time.Date(2025, time.January, 2, 3, 4, 0, 0, time.UTC)
	items := historyMedia([]storage.HistoryEntry{
		{ID: "tt1", Title: "Movie", Type: "movie", PlayedAt: playedAt},
		{ID: "bad", Title: "Invalid", Type: "podcast"},
		{ID: "tt2", Title: "Series", Type: "series"},
	})
	if len(items) != 2 || items[0].Name != "Movie" || items[1].Type != model.Series {
		t.Fatalf("items = %#v", items)
	}
	if !items[0].PlayedAt.Equal(playedAt) {
		t.Fatalf("played at = %v", items[0].PlayedAt)
	}
}

func TestHistoryChoiceHasDatePlayedMode(t *testing.T) {
	playedAt := time.Date(2025, time.January, 2, 3, 4, 0, 0, time.Local)
	modes := (navigationChoice{kind: navigationMedia, playedAt: playedAt}).ContextModes()
	last := modes[len(modes)-1]
	if last.Key != "p" || last.Name != "Date played" || last.Value != "2025-01-02" {
		t.Fatalf("date played mode = %#v", last)
	}
}

func TestFutureEpisodeIsUnavailable(t *testing.T) {
	future := navigationChoice{kind: navigationEpisode, episode: model.Episode{Released: time.Now().Add(time.Hour)}}
	past := navigationChoice{kind: navigationEpisode, episode: model.Episode{Released: time.Now().Add(-time.Hour)}}
	if !future.Unavailable() || past.Unavailable() {
		t.Fatalf("availability: future=%t past=%t", future.Unavailable(), past.Unavailable())
	}
}

func TestNavigationWatchIdentities(t *testing.T) {
	media := model.Media{ID: "tt1", Type: model.Series, Name: "Silo"}
	episodes := []model.Episode{{Season: 1, Episode: 1}, {Season: 1, Episode: 2}}
	identity, keys := (navigationChoice{kind: navigationSeason, media: media, episodes: episodes}).WatchIdentity()
	if identity != "tt1" || strings.Join(keys, ",") != "1:1,1:2" {
		t.Fatalf("season identity = %q, %#v", identity, keys)
	}
	streams, err := streamChoices(media, episodes[0], []model.Stream{{Title: "stream"}}, nil)
	if err != nil || len(streams) != 1 || streams[0].episode.Episode != 1 {
		t.Fatalf("stream episode origin = %#v, %v", streams, err)
	}
}

func TestNavigationDetailModeDefaults(t *testing.T) {
	cases := []struct {
		choice navigationChoice
		key    string
	}{
		{choice: navigationChoice{kind: navigationMedia}, key: "y"},
		{choice: navigationChoice{kind: navigationSeason}, key: "e"},
		{choice: navigationChoice{kind: navigationEpisode}, key: "a"},
		{choice: navigationChoice{kind: navigationStream}, key: "q"},
	}
	for _, test := range cases {
		modes := test.choice.ContextModes()
		if len(modes) == 0 || modes[0].Key != test.key {
			t.Fatalf("default mode = %#v, want %q", modes, test.key)
		}
	}
}

func TestApplyPlayerPreferenceImmediately(t *testing.T) {
	fallback := player.Player{Executable: "xdg-open", Arguments: []string{"--default"}}
	active := fallback
	applyPlayerPreference(&active, fallback, false, "vlc")
	if active.Executable != "vlc" || len(active.Arguments) != 0 {
		t.Fatalf("custom player = %#v", active)
	}
	applyPlayerPreference(&active, fallback, false, "")
	if active.Executable != "xdg-open" || len(active.Arguments) != 1 {
		t.Fatalf("restored player = %#v", active)
	}
}

func TestApplyPlayerPreferenceRespectsEnvironmentOverride(t *testing.T) {
	active := player.Player{Executable: "environment-player"}
	applyPlayerPreference(&active, player.Player{Executable: "xdg-open"}, true, "vlc")
	if active.Executable != "environment-player" {
		t.Fatalf("player = %#v", active)
	}
}

func TestSetTorBoxTokenUpdatesProviderAndClient(t *testing.T) {
	a := App{
		TorBox: torbox.Client{},
		Providers: map[string]provider.Provider{
			provider.TorBoxID: provider.TorBox{TorBoxClient: torbox.Client{}},
		},
	}

	if err := a.setTorBoxToken("saved-token"); err != nil {
		t.Fatal(err)
	}
	configured := a.Providers[provider.TorBoxID].(provider.TorBox)
	if a.TorBox.Token != "saved-token" || configured.TorBoxClient.Token != "saved-token" {
		t.Fatalf("tokens = app %q, provider %q", a.TorBox.Token, configured.TorBoxClient.Token)
	}
}
