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
	items, err := a.Search(context.Background(), "Dune", model.Movie)
	if err != nil || len(items) != 1 {
		t.Fatalf("search = %#v, %v", items, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	items, err = a.Search(context.Background(), "Dune", model.Movie)
	if err != nil || len(items) != 1 {
		t.Fatalf("reopened search = %#v, %v", items, err)
	}
	if requests != 1 {
		t.Fatalf("catalog requests = %d", requests)
	}
}

func TestSeriesEpisodesUsePersistentCache(t *testing.T) {
	requests := 0
	fail := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests++
		if fail {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
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
	episodes, err := a.seriesEpisodes(context.Background(), "tt1", false)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("episodes = %#v, %v", episodes, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	episodes, err = a.seriesEpisodes(context.Background(), "tt1", false)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("reopened episodes = %#v, %v", episodes, err)
	}
	fail = true
	if _, err := a.seriesEpisodes(context.Background(), "tt1", true); err == nil {
		t.Fatal("failed refresh succeeded")
	}
	episodes, err = a.seriesEpisodes(context.Background(), "tt1", false)
	if err != nil || len(episodes) != 1 {
		t.Fatalf("cache lost after failed refresh = %#v, %v", episodes, err)
	}
	if requests != 2 {
		t.Fatalf("catalog requests = %d", requests)
	}
}

func TestCatalogCacheTTLs(t *testing.T) {
	if searchCacheTTL != 24*time.Hour || seriesCacheTTL != 30*24*time.Hour {
		t.Fatalf("cache TTLs = %v, %v", searchCacheTTL, seriesCacheTTL)
	}
}

func TestNavigationCacheKeysCoverSeriesMoviesAndEpisodes(t *testing.T) {
	series := navigationChoice{kind: navigationMedia, media: model.Media{ID: "tt1", Type: model.Series}}
	movie := navigationChoice{kind: navigationMedia, media: model.Media{ID: "tt2", Type: model.Movie}}
	episode := navigationChoice{kind: navigationEpisode, episode: model.Episode{ID: "tt1:1:1"}}
	if series.CacheKey() != "series:tt1" || movie.CacheKey() != "streams:tt2" || episode.CacheKey() != "streams:tt1:1:1" {
		t.Fatalf("cache keys = %q, %q, %q", series.CacheKey(), movie.CacheKey(), episode.CacheKey())
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

func TestPossibleEpisodeUpdateUsesNumericMaximumAiredEpisode(t *testing.T) {
	now := time.Date(2026, time.September, 12, 0, 0, 0, 0, time.UTC)
	episodes := []model.Episode{
		{Season: 1, Episode: 10, Released: now.Add(-24 * time.Hour)},
		{Season: 1, Episode: 11, Released: now.Add(24 * time.Hour)},
	}
	latest, ok := possibleEpisodeUpdate([]string{"1:9"}, episodes, now)
	if !ok || latest.Season != 1 || latest.Episode != 10 {
		t.Fatalf("update = %#v, %t", latest, ok)
	}
	if _, ok := possibleEpisodeUpdate([]string{"1:10"}, episodes, now); ok {
		t.Fatal("future episode triggered update")
	}
	if _, ok := possibleEpisodeUpdate(nil, episodes, now); ok {
		t.Fatal("series without episode baseline triggered update")
	}
}

func TestHistoryMediaMarksUpdatesFromSeriesCache(t *testing.T) {
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a := App{Catalog: catalog.Client{BaseURL: "https://catalog.example"}, Storage: store}
	if err := store.RecordHistory(storage.HistoryEntry{ID: "tt1", Title: "Series", Type: "series", Episodes: []string{"2:8"}}); err != nil {
		t.Fatal(err)
	}
	episodes := []model.Episode{{Season: 2, Episode: 9, Released: time.Now().Add(-time.Hour)}}
	if err := store.CachePut(storage.CacheSeries, a.seriesCacheKey("tt1"), episodes, seriesCacheTTL); err != nil {
		t.Fatal(err)
	}
	items, err := a.loadHistoryMedia()
	if err != nil || len(items) != 1 || items[0].UpdateEpisode != "2:9" {
		t.Fatalf("history = %#v, %v", items, err)
	}
}

func TestHistoryMediaMarksUpdateAfterFirstWatchedEpisode(t *testing.T) {
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	a := App{Catalog: catalog.Client{BaseURL: "https://catalog.example"}, Storage: store}
	entry := storage.HistoryEntry{ID: "tt1", Title: "Series", Type: "series"}
	if err := store.RecordHistory(entry); err != nil {
		t.Fatal(err)
	}
	episodes := []model.Episode{
		{Season: 1, Episode: 1, Released: time.Now().Add(-2 * time.Hour)},
		{Season: 1, Episode: 2, Released: time.Now().Add(-time.Hour)},
	}
	if err := store.CachePut(storage.CacheSeries, a.seriesCacheKey("tt1"), episodes, seriesCacheTTL); err != nil {
		t.Fatal(err)
	}
	if items, err := a.loadHistoryMedia(); err != nil || items[0].UpdateEpisode != "" {
		t.Fatalf("history before episode watch = %#v, %v", items, err)
	}
	watched, err := store.ToggleWatched(entry, []string{"1:1"})
	if err != nil {
		t.Fatal(err)
	}
	items, err := a.loadHistoryMedia()
	if err != nil || items[0].UpdateEpisode != "1:2" {
		t.Fatalf("history after episode watch = %#v, %v", items, err)
	}
	choice := navigationChoice{kind: navigationMedia, media: items[0]}
	if choice.Status(map[string]bool(watched)) != "+" {
		t.Fatal("possible update status did not appear after episode watch")
	}
}

func TestEpisodeUpdateStatusTracksLiveWatchedState(t *testing.T) {
	choice := navigationChoice{kind: navigationMedia, media: model.Media{ID: "tt1", Type: model.Series, UpdateEpisode: "2:9"}}
	if choice.Status(nil) != "+" {
		t.Fatal("possible update status missing")
	}
	if choice.Status(map[string]bool{"tt1:2:9": true}) != "" {
		t.Fatal("watched update retained status")
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

func TestHistorySelectionCombinesPreviousSeasons(t *testing.T) {
	media := model.Media{ID: "tt1", Type: model.Series, Name: "Silo"}
	selected := []navigationChoice{
		{kind: navigationSeason, media: media, episodes: []model.Episode{{Season: 1, Episode: 1}, {Season: 1, Episode: 2}}},
		{kind: navigationSeason, media: media, episodes: []model.Episode{{Season: 2, Episode: 1}}},
	}
	entry, keys, err := historySelection(selected)
	if err != nil || entry.ID != "tt1" || strings.Join(keys, ",") != "1:1,1:2,2:1" {
		t.Fatalf("history selection = %#v, %#v, %v", entry, keys, err)
	}
	if !selected[0].WatchThrough() || !(navigationChoice{kind: navigationEpisode}).WatchThrough() || (navigationChoice{kind: navigationMedia}).WatchThrough() || (navigationChoice{kind: navigationStream}).WatchThrough() {
		t.Fatal("watched-through pane eligibility is incorrect")
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
