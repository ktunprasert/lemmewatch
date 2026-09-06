package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/config"
	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/provider"
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

func TestHistoryMediaPreservesPlayableEntries(t *testing.T) {
	playedAt := time.Date(2025, time.January, 2, 3, 4, 0, 0, time.UTC)
	items := historyMedia([]config.HistoryEntry{
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
