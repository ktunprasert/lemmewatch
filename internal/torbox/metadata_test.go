package torbox

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lemmewatch/internal/metadata"
)

func TestCacheAndAccountMetadataRetainsExtraFields(t *testing.T) {
	hash := strings.Repeat("a", 40)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/torrents/checkcached":
			fmt.Fprintf(w, `{"success":true,"data":{"%s":{"name":"Release","files":[{"id":7,"mimetype":"video/x-matroska","extra":{"codec":"hevc"}}]}}}`, hash)
		case "/torrents/mylist":
			fmt.Fprintf(w, `{"success":true,"data":[{"hash":"%s","download_state":"cached","ratio":2.5,"files":[{"id":7,"new_property":"retained"}],"auth_token":"private"}]}`, hash)
		default:
			t.Errorf("info mutated account: %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	c := Client{BaseURL: server.URL, HTTP: server.Client(), Token: "test"}
	items, err := c.CachedDetails(context.Background(), []string{hash})
	if err != nil || !items[hash].Cached {
		t.Fatalf("cache = %v, %v", items, err)
	}
	text := strings.Join(metadata.Lines("Cache", items[hash].Metadata), "\n")
	if !strings.Contains(text, "Mimetype: video/x-matroska") || !strings.Contains(text, "Codec: hevc") {
		t.Fatal(text)
	}
	fields, err := c.TorrentMetadata(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	text = strings.Join(metadata.Lines("Account", fields), "\n")
	if !strings.Contains(text, "Ratio: 2.5") || !strings.Contains(text, "New property: retained") || strings.Contains(text, "private") {
		t.Fatal(text)
	}
	fields, err = c.TorrentMetadata(context.Background(), strings.Repeat("b", 40))
	if err != nil || len(fields) != 0 {
		t.Fatalf("missing torrent = %v, %v", fields, err)
	}
}
