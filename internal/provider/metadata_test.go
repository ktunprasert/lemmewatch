package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"lemmewatch/internal/metadata"
	"lemmewatch/internal/model"
	"lemmewatch/internal/storage"
	"lemmewatch/internal/stremio"
	"lemmewatch/internal/torbox"
)

func TestTorBoxInfoRetainsAddonAndCacheMetadata(t *testing.T) {
	hash := strings.Repeat("a", 40)
	addonRequests := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/stream/movie/tt1.json":
			addonRequests++
			fmt.Fprintf(w, `{"streams":[{"infoHash":"%s","name":"Torrentio","title":"Release","extra":{"codec":"hevc"}}]}`, hash)
		case "/torrents/checkcached":
			fmt.Fprintf(w, `{"success":true,"data":{"%s":{"files":[{"id":7,"mimetype":"video/x-matroska"}]}}}`, hash)
		case "/torrents/mylist":
			fmt.Fprintf(w, `{"success":true,"data":[{"hash":"%s","ratio":2.5}]}`, hash)
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	store := storage.NewAt(filepath.Join(root, "history.db"), filepath.Join(root, "cache.db"), filepath.Join(root, "history.json"))
	if err := store.Open(); err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	p := TorBox{StreamsClient: stremio.Client{BaseURL: server.URL, HTTP: server.Client()}, TorBoxClient: torbox.Client{BaseURL: server.URL, HTTP: server.Client(), Token: "test"}, Storage: store}
	for range 2 {
		streams, err := p.Streams(context.Background(), Request{MediaType: model.Movie, ID: "tt1"})
		if err != nil || len(streams) != 1 {
			t.Fatalf("streams = %v, %v", streams, err)
		}
		stream := streams[0]
		if !strings.Contains(strings.Join(metadata.Lines("Addon", stream.Metadata), "\n"), "Codec: hevc") {
			t.Fatal("cached candidate lost extra fields")
		}
		if !strings.Contains(strings.Join(metadata.Lines("Cache", stream.CacheMetadata), "\n"), "Mimetype: video/x-matroska") {
			t.Fatal("cache metadata missing")
		}
		full, err := p.Info(context.Background(), stream)
		if err != nil || string(full.TorrentMetadata["ratio"]) != "2.5" {
			t.Fatalf("info = %#v, %v", full, err)
		}
	}
	if addonRequests != 1 {
		t.Fatalf("addon requests = %d", addonRequests)
	}
}
