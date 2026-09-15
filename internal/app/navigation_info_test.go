package app

import (
	"strings"
	"testing"
	"time"

	"lemmewatch/internal/metadata"
	"lemmewatch/internal/model"
)

func TestNavigationInfoUsesAvailableMetadata(t *testing.T) {
	released := time.Date(2024, 7, 9, 0, 0, 0, 0, time.UTC)
	media := model.Media{ID: "tt1", Name: "Show", Type: model.Series, Year: 2024, Rating: "8.1", Summary: "A full summary.", PlayedAt: released}
	episode := model.Episode{Season: 1, Episode: 6, Title: "Full episode title", Released: released, Rating: "8.4"}
	watched := map[string]bool{"tt1:1:6": true}
	tests := []struct {
		name string
		item navigationChoice
		want []string
	}{
		{"media", navigationChoice{kind: navigationMedia, media: media}, []string{"Show", "Series · 2024 · Rating 8.1", "Last played:", "A full summary."}},
		{"season", navigationChoice{kind: navigationSeason, media: media, season: 1, episodes: []model.Episode{episode, {Season: 1, Episode: 7, Released: released.AddDate(0, 0, 7)}, {Season: 1, Episode: 8}}}, []string{"Season 1 · 3 episodes", "Watched: 1/3", "Released: 2024-07-09 – 2024-07-16", "Show"}},
		{"episode", navigationChoice{kind: navigationEpisode, media: media, episode: episode}, []string{"Full episode title", "S01E06 · Watched", "Released: 2024-07-09 · Rating 8.4", "Show"}},
		{"stream", navigationChoice{kind: navigationStream, stream: model.Stream{Title: "Full release title", Filename: "episode.mkv", Quality: 1080, Size: 1_500_000_000, Seeders: 23, Cache: model.CacheCached, Provider: "TorBox", Source: "Torrentio", Playable: true}}, []string{"Full release title", "1080p · 1.50 GB · 23 seeders", "Cached · TorBox · Torrentio", "File: episode.mkv"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			text := strings.Join(tt.item.InfoLines(watched), "\n")
			for _, expected := range tt.want {
				if !strings.Contains(text, expected) {
					t.Fatalf("missing %q in %q", expected, text)
				}
			}
		})
	}
}

func TestInfoPanelsShowUsefulFieldsWithoutRawNoise(t *testing.T) {
	media := model.Media{ID: "tt1", Name: "The Paper", Type: model.Series, Rating: "7.0", Summary: "A newspaper documentary.", Metadata: metadata.Parse([]byte(`{"genres":["Comedy"],"genre":["Comedy"],"cast":["One","Two","Three","Four","Five","Six","Seven"],"runtime":"30 min","country":"United States","status":"Returning Series","background":"https://example.invalid/image","imdb_id":"tt1","links":[{"category":"share","name":"The Paper"}],"behaviorHints":{"hasScheduledVideos":true}}`))}
	text := strings.Join((navigationChoice{kind: navigationMedia, media: media}).InfoLines(nil), "\n")
	for _, want := range []string{"Genres: Comedy", "Rating 7.0", "30 min", "A newspaper documentary.", "Cast: One, Two, Three, Four, Five, Six (+1)"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q: %s", want, text)
		}
	}
	if strings.Count(text, "Comedy") != 1 {
		t.Fatal("duplicate genres")
	}
	for _, noise := range []string{"[url omitted]", "Cinemeta:", "Behavior", "tt1", "Links", "Seven"} {
		if strings.Contains(text, noise) {
			t.Fatalf("noise %q: %s", noise, text)
		}
	}
	season := navigationChoice{kind: navigationSeason, media: media, season: 1, episodes: []model.Episode{{Season: 1, Episode: 1}}}
	text = strings.Join(season.InfoLines(nil), "\n")
	if strings.Contains(text, "Cast") || strings.Contains(text, "documentary") || strings.Contains(text, "Genres") {
		t.Fatalf("season repeats show details: %s", text)
	}
	episode := navigationChoice{kind: navigationEpisode, media: media, episode: model.Episode{Title: "Pilot", Season: 1, Episode: 1, Metadata: metadata.Parse([]byte(`{"description":"The crew finds a newspaper.","overview":"The crew finds a newspaper.","firstAired":"2025-09-04T11:00:00.000Z","number":1,"id":"tt1:1:1"}`))}}
	text = strings.Join(episode.InfoLines(nil), "\n")
	if strings.Count(text, "The crew finds a newspaper.") != 1 || strings.Contains(text, "First Aired") || strings.Contains(text, "Id:") {
		t.Fatalf("episode duplicates: %s", text)
	}
	stream := navigationChoice{kind: navigationStream, stream: model.Stream{Title: "Release", Filename: "pilot.mkv", Quality: 1080, Cache: model.CacheCached, Metadata: metadata.Parse([]byte(`{"infoHash":"raw-hash","title":"Release","behaviorHints":{"bingeGroup":"internal"}}`)), CacheMetadata: metadata.Parse([]byte(`{"name":"Season pack","hash":"raw-hash","size":7000000000,"files":[{"short_name":"pilot.mkv","mimetype":"video/x-matroska"},{"short_name":"episode2.mkv"}]}`))}}
	text = strings.Join(stream.InfoLines(nil), "\n")
	for _, want := range []string{"Pack: 2 files", "Format: Matroska", "File: pilot.mkv"} {
		if !strings.Contains(text, want) {
			t.Fatal(text)
		}
	}
	for _, noise := range []string{"raw-hash", "Binge", "7000000000", "episode2.mkv", "Stream addon:", "TorBox cache:"} {
		if strings.Contains(text, noise) {
			t.Fatalf("stream noise %q: %s", noise, text)
		}
	}
}

func TestNavigationInfoOmitsUnknownAndTorrentOnlyMetadata(t *testing.T) {
	media := navigationChoice{kind: navigationMedia, media: model.Media{Name: "Movie", Type: model.Movie}}
	text := strings.Join(media.InfoLines(nil), "\n")
	for _, absent := range []string{"Rating", "Last played", "0001", " · "} {
		if strings.Contains(text, absent) {
			t.Fatalf("unknown metadata rendered: %q", text)
		}
	}
	direct := navigationChoice{kind: navigationStream, stream: model.Stream{Title: "Direct title", Cache: model.CacheNotApplicable, Playable: true, Provider: "Pengu"}}
	text = strings.Join(direct.InfoLines(nil), "\n")
	if !strings.Contains(text, "Direct · Pengu") || strings.Contains(text, "seeders") || strings.Contains(text, "GB") || strings.Contains(text, "File:") {
		t.Fatalf("direct metadata = %q", text)
	}
}

func TestEpisodeInfoReflectsWatchedUpdates(t *testing.T) {
	episode := navigationChoice{kind: navigationEpisode, media: model.Media{ID: "tt1"}, episode: model.Episode{Season: 2, Episode: 10}}
	watched := map[string]bool{}
	if !strings.Contains(strings.Join(episode.InfoLines(watched), "\n"), "S02E10 · Unwatched") {
		t.Fatal("unwatched state missing")
	}
	watched["tt1:2:10"] = true
	if !strings.Contains(strings.Join(episode.InfoLines(watched), "\n"), "S02E10 · Watched") {
		t.Fatal("watched state stale")
	}
}
