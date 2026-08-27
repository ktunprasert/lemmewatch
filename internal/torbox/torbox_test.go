package torbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVideoFilesPreserveAddonIndex(t *testing.T) {
	torrent := torrent{Files: []file{{ID: 1, Name: "poster.jpg"}, {ID: 2, Name: "movie.mkv"}, {ID: 3, Name: "sample.txt"}, {ID: 4, Name: "bonus.MP4"}}}
	files := torrent.videoFiles()
	if len(files) != 2 || files[0].ID != 2 || files[1].ID != 4 {
		t.Fatalf("files = %#v", files)
	}
}

func TestCachedDeduplicatesHashesAndSetsHeaders(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method = %q", r.Method)
		}
		var body struct {
			Hashes []string `json:"hashes"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if len(body.Hashes) != 1 || body.Hashes[0] != hash {
			t.Errorf("hashes = %#v", body.Hashes)
		}
		if got := r.Header.Get("User-Agent"); got != "lemmewatch/0.1" {
			t.Errorf("User-Agent = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Errorf("Authorization = %q", got)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"` + hash + `":true}}`))
	}))
	defer server.Close()

	result, err := (Client{BaseURL: server.URL, Token: "token", HTTP: server.Client()}).Cached(context.Background(), []string{hash, hash})
	if err != nil {
		t.Fatal(err)
	}
	if !result[hash] {
		t.Fatalf("result = %#v", result)
	}
}
