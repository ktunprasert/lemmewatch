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
			`{"name":"Torrentio 1080p","title":"👤 50 2.5 GB","infoHash":"` + hashA + `","fileIdx":1},` +
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
	if items[0].Quality != 2160 || items[0].Hash != hashB {
		t.Fatalf("ranking = %#v", items)
	}
	if items[1].Seeders != 50 || items[1].Size != 2_500_000_000 {
		t.Fatalf("parsing = %#v", items[1])
	}
}
