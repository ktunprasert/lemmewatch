package stremio

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamsNormalizesDeduplicatesAndRanks(t *testing.T) {
	hashA := "0123456789abcdef0123456789abcdef01234567"
	hashB := "abcdef0123456789abcdef0123456789abcdef01"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("User-Agent"); got != "lemmewatch/0.1" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Accept"); got != "application/json" {
			t.Errorf("Accept = %q", got)
		}
		_, _ = w.Write([]byte(`{"streams":[` +
			`{"name":"Torrentio 1080p","title":"Episode.Release.1080p 👤\n👤 50 💾 2.5 GB ⚙️ source\n🇬🇧 / 🇮🇹","infoHash":"` + hashA + `","fileIdx":1,"behaviorHints":{"filename":"episode.mkv"}},` +
			`{"name":"duplicate","title":"x","infoHash":"` + hashA + `","fileIdx":1},` +
			`{"name":"Torrentio 2160p","title":"Seeders: 8 10 GB","url":"magnet:?xt=urn:btih:` + hashB + `"},` +
			`{"name":"bad","infoHash":"nope"}]}`))
	}))
	defer server.Close()
	items, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Streams(context.Background(), "tt1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v", items)
	}
	Rank(items)
	if items[0].Quality != 2160 || items[0].Hash != hashB {
		t.Fatalf("ranking = %#v", items)
	}
	if items[1].Seeders != 50 || items[1].Size != 2_500_000_000 {
		t.Fatalf("parsing = %#v", items[1])
	}
	if items[1].Filename != "episode.mkv" {
		t.Fatalf("filename = %q", items[1].Filename)
	}
	if items[1].Title != "Episode.Release.1080p" {
		t.Fatalf("title = %q", items[1].Title)
	}
}

func TestSeriesStreamsUsesEpisodeRoute(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/stream/series/tt123:2:3.json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"streams":[]}`))
	}))
	defer server.Close()
	_, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).SeriesStreams(context.Background(), "tt123:2:3")
	if err != nil {
		t.Fatal(err)
	}
}

func TestStreamsPreservesDirectHTTPMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"streams":[` +
			`{"name":"WebStreamr 1080p","title":"Direct 1080p","url":"https://media.example/video","behaviorHints":{"notWebReady":true,"videoSize":1234}},` +
			`{"name":"headers","url":"https://media.example/protected","behaviorHints":{"proxyHeaders":{"request":{"Referer":"https://source.example/"}}}},` +
			`{"name":"unsafe","url":"javascript:alert(1)"}]}`))
	}))
	defer server.Close()
	items, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Streams(context.Background(), "tt1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].URL != "https://media.example/video" || items[0].Size != 1234 || !items[0].NotWebReady {
		t.Fatalf("direct streams = %#v", items)
	}
	if items[1].Headers["Referer"] != "https://source.example/" {
		t.Fatalf("headers = %#v", items[1].Headers)
	}
}

func TestStreamsUsesDescriptionMetadata(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"streams":[{"name":"PenguPlay 1080p","description":"Dune (2021)\n1080p • MP4 • WEBRip • 9.6 Mbps\n10.34 GB","url":"https://media.example/video"}]}`))
	}))
	defer server.Close()
	items, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Streams(context.Background(), "tt1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Title != "1080p • MP4 • WEBRip • 9.6 Mbps" || items[0].Quality != 1080 || items[0].Size != 10_340_000_000 {
		t.Fatalf("items = %#v", items)
	}
}

func TestStreamsRecognizes360p(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"streams":[{"name":"PenguPlay 360p","url":"https://media.example/video"}]}`))
	}))
	defer server.Close()
	items, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Streams(context.Background(), "tt1")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Quality != 360 {
		t.Fatalf("items = %#v", items)
	}
}

func TestStreamsAcceptsConfiguredManifestURL(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/%7B%22proxy%22%3A%22https%3A%2F%2Fexample.invalid%22%7D/stream/movie/tt1.json" {
			t.Errorf("path = %q", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"streams":[]}`))
	}))
	defer server.Close()
	base := server.URL + "/%7B%22proxy%22%3A%22https%3A%2F%2Fexample.invalid%22%7D/manifest.json"
	if _, err := (Client{BaseURL: base, HTTP: server.Client()}).Streams(context.Background(), "tt1"); err != nil {
		t.Fatal(err)
	}
}
