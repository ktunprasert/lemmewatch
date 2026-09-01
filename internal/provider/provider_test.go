package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"lemmewatch/internal/model"
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
