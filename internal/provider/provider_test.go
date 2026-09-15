package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	bolt "go.etcd.io/bbolt"

	"lemmewatch/internal/model"
	"lemmewatch/internal/storage"
	"lemmewatch/internal/stremio"
	"lemmewatch/internal/torbox"
)

func TestWebStreamrReturnsPlayableDirectStreams(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stream/movie/tt1160419.json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"streams":[{"name":"WebStreamr 1080p","title":"Dune 1080p","url":"https://resolver.example/extract?id=secret"}]}`))
	}))
	defer server.Close()
	p := WebStreamr{Client: stremio.Client{BaseURL: server.URL, HTTP: server.Client()}}
	streams, err := p.Streams(context.Background(), Request{MediaType: model.Movie, ID: "tt1160419"})
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 || streams[0].Provider != WebStreamrID || !streams[0].Playable || streams[0].Cache != model.CacheNotApplicable {
		t.Fatalf("streams = %#v", streams)
	}
	p.Client.HTTP = nil
	playback, err := p.Resolve(context.Background(), streams[0])
	if err != nil || playback.URL != streams[0].URL {
		t.Fatalf("playback = %#v, %v", playback, err)
	}
}

func TestWebStreamrUsesSeriesEpisodeRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stream/series/tt0944947:1:1.json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"streams":[{"name":"WebStreamr","url":"https://resolver.example/video"}]}`))
	}))
	defer server.Close()
	p := WebStreamr{Client: stremio.Client{BaseURL: server.URL, HTTP: server.Client()}}
	streams, err := p.Streams(context.Background(), Request{MediaType: model.Series, ID: "tt0944947:1:1", Season: 1, Episode: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 || streams[0].Season != 1 || streams[0].Episode != 1 {
		t.Fatalf("streams = %#v", streams)
	}
}

func TestWebStreamrRejectsHeaderDependentPlayback(t *testing.T) {
	p := WebStreamr{}
	_, err := p.Resolve(context.Background(), model.Stream{Provider: WebStreamrID, URL: "https://example.invalid/video", Headers: map[string]string{"Referer": "https://example.invalid/"}})
	if err == nil {
		t.Fatal("header-dependent stream resolved")
	}
}

func TestPenguUsesConfiguredManifestAndMarksHeadersUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/secret-config/stream/movie/tt1160419.json" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"streams":[{"name":"PenguPlay 1080p","url":"https://media.example/video"},{"name":"PenguPlay 720p","url":"https://media.example/protected","behaviorHints":{"proxyHeaders":{"request":{"Referer":"https://source.example/"}}}}]}`))
	}))
	defer server.Close()
	p := Pengu{Client: stremio.Client{BaseURL: server.URL + "/secret-config/manifest.json", HTTP: server.Client()}}
	streams, err := p.Streams(context.Background(), Request{MediaType: model.Movie, ID: "tt1160419"})
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 2 || streams[0].Provider != PenguID || !streams[0].Playable || streams[1].Playable {
		t.Fatalf("streams = %#v", streams)
	}
	_, err = p.Resolve(context.Background(), streams[1])
	if err == nil {
		t.Fatal("header-dependent Pengu stream resolved")
	}
}

func TestWebStreamrUnwrapsDownloadRedirect(t *testing.T) {
	media := "https://video-downloads.googleusercontent.com/video"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://gamerxyt.com/dl.php?link="+url.QueryEscape(media), http.StatusFound)
	}))
	defer server.Close()
	p := WebStreamr{Client: stremio.Client{HTTP: server.Client()}}
	playback, err := p.Resolve(context.Background(), model.Stream{Provider: WebStreamrID, URL: server.URL})
	if err != nil {
		t.Fatal(err)
	}
	if playback.URL != media {
		t.Fatalf("URL = %q", playback.URL)
	}
}

func TestUnwrapDownloadURLRejectsUnknownWrapper(t *testing.T) {
	if got := unwrapDownloadURL("https://example.invalid/dl.php?link=https://media.example/video"); got != "" {
		t.Fatalf("URL = %q", got)
	}
}

func TestTorBoxProviderOwnsCacheEnrichment(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/movie/tt1.json":
			_, _ = w.Write([]byte(`{"streams":[{"name":"Torrentio 1080p","title":"Release 1080p","infoHash":"` + hash + `","fileIdx":0}]}`))
		case "/torrents/checkcached":
			_, _ = w.Write([]byte(`{"success":true,"data":{"` + hash + `":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p := TorBox{
		StreamsClient: stremio.Client{BaseURL: server.URL, HTTP: server.Client()},
		TorBoxClient:  torbox.Client{BaseURL: server.URL, Token: "token", HTTP: server.Client()},
	}
	streams, err := p.Streams(context.Background(), Request{MediaType: model.Movie, ID: "tt1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(streams) != 1 || streams[0].Provider != TorBoxID || streams[0].Cache != model.CacheCached || !streams[0].Playable {
		t.Fatalf("streams = %#v", streams)
	}
}

func TestTorBoxCachesOnlyStableCandidatesAndRefreshesAvailability(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	streamRequests := 0
	cacheRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/movie/tt1.json":
			streamRequests++
			_, _ = w.Write([]byte(`{"streams":[{"name":"Torrentio 1080p","title":"Release 1080p\n🇯🇵\nAudio: Japanese\nSubs: English","infoHash":"` + hash + `","fileIdx":0,"url":"https://signed.example/secret","behaviorHints":{"proxyHeaders":{"request":{"Authorization":"secret"}}}}]}`))
		case "/torrents/checkcached":
			cacheRequests++
			_, _ = w.Write([]byte(`{"success":true,"data":{"` + hash + `":true}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	cachePath := filepath.Join(root, "cache.db")
	store := storage.NewAt(filepath.Join(root, "history.db"), cachePath, filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	p := TorBox{
		StreamsClient: stremio.Client{BaseURL: server.URL, HTTP: server.Client()},
		TorBoxClient:  torbox.Client{BaseURL: server.URL, Token: "token", HTTP: server.Client()},
		Storage:       store,
	}
	request := Request{MediaType: model.Movie, ID: "tt1"}
	if _, err := p.Streams(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	if streams, err := p.Streams(context.Background(), request); err != nil {
		t.Fatal(err)
	} else if len(streams[0].AudioLanguages) != 1 || streams[0].AudioLanguages[0] != "ja" || len(streams[0].SubtitleLanguages) != 1 || streams[0].SubtitleLanguages[0] != "en" || len(streams[0].LanguageHints) != 1 || streams[0].LanguageHints[0] != "ja" {
		t.Fatalf("cache lost language metadata: %#v", streams[0])
	}
	request.Refresh = true
	if _, err := p.Streams(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if streamRequests != 2 || cacheRequests != 3 {
		t.Fatalf("requests = stream %d, cache %d", streamRequests, cacheRequests)
	}
	key := "v3:" + storage.SourceFingerprint(server.URL) + ":movie:tt1"
	var candidates []torrentCandidate
	hit, err := store.CacheGet(storage.CacheTorrents, key, &candidates)
	if err != nil || !hit || len(candidates) != 1 {
		t.Fatalf("candidate cache = %t, %#v, %v", hit, candidates, err)
	}
	if strings.Contains(candidates[0].Title+candidates[0].Filename+candidates[0].Source, "secret") {
		t.Fatalf("cached candidate contains secret: %#v", candidates[0])
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	database, err := bolt.Open(cachePath, 0o600, &bolt.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	var raw string
	if err := database.View(func(tx *bolt.Tx) error {
		bucket := tx.Bucket([]byte("cache:" + storage.CacheTorrents))
		if bucket != nil {
			raw = string(bucket.Get([]byte(key)))
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if raw == "" {
		t.Fatal("torrent cache entry missing")
	}
	if strings.Contains(raw, "signed.example") || strings.Contains(raw, "Authorization") || strings.Contains(raw, "secret") {
		t.Fatalf("cache persisted URL or credentials: %s", raw)
	}
}

func TestTorBoxQueueTorrentWaitsForDownload(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrents/createtorrent":
			_, _ = w.Write([]byte(`{"success":true,"data":{"torrent_id":7}}`))
		case "/torrents/mylist":
			if r.URL.Query().Get("id") == "" {
				_, _ = w.Write([]byte(`{"success":true,"data":[]}`))
				return
			}
			polls++
			progress := 0.25
			if polls >= 2 {
				progress = 1
			}
			_, _ = w.Write([]byte(`{"success":true,"data":{"id":7,"progress":` + strconv.FormatFloat(progress, 'f', -1, 64) + `}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	p := TorBox{TorBoxClient: torbox.Client{BaseURL: server.URL, Token: "token", HTTP: server.Client(), PollInterval: time.Millisecond}}

	var updates []QueueProgress
	err := p.QueueTorrent(context.Background(), model.Stream{Provider: TorBoxID, Hash: hash}, func(p QueueProgress) {
		updates = append(updates, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if polls < 2 || len(updates) == 0 || updates[0].TorrentID != 7 {
		t.Fatalf("polls = %d, updates = %#v", polls, updates)
	}
}
