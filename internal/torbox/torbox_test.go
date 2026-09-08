package torbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestVideoFilesPreserveAddonIndex(t *testing.T) {
	torrent := torrent{Files: []file{{ID: 1, Name: "poster.jpg"}, {ID: 2, Name: "movie.mkv"}, {ID: 3, Name: "sample.txt"}, {ID: 4, Name: "bonus.MP4"}}}
	files := torrent.videoFiles()
	if len(files) != 2 || files[0].ID != 2 || files[1].ID != 4 {
		t.Fatalf("files = %#v", files)
	}
}

func TestSelectVideoFilePrefersExactAddonFilename(t *testing.T) {
	filename := "Family Guy (1999) - S01E01 - Death Has a Shadow [DSNP WEBDL-1080p][AAC 2.0][h264]-PHOENiX.mkv"
	files := []file{
		{ID: 127, Name: "Family Guy (1999)/Season 01/" + filename, ShortName: filename},
		{ID: 110, Name: "Family Guy (1999)/Season 09/Family Guy (1999) - S09E18 - Its a Trap.mkv", ShortName: "Family Guy (1999) - S09E18 - Its a Trap.mkv"},
	}
	selected, err := selectVideoFile(files, 165, filename, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != 127 {
		t.Fatalf("selected ID = %d", selected.ID)
	}
}

func TestSelectVideoFileMatchesEpisodeWhenFilenameMissing(t *testing.T) {
	files := []file{
		{ID: 1, Name: "Show - S01E01 sample.mkv", Size: 10},
		{ID: 2, Name: "Show - S01E01 proper.mkv", Size: 100},
		{ID: 3, Name: "Show - S19E18.mkv", Size: 1000},
	}
	selected, err := selectVideoFile(files, 2, "", 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != 2 {
		t.Fatalf("selected ID = %d", selected.ID)
	}
}

func TestSelectVideoFileFallsBackToVideoIndex(t *testing.T) {
	files := []file{{ID: 1, Name: "first.mkv"}, {ID: 2, Name: "second.mkv"}}
	selected, err := selectVideoFile(files, 1, "missing.mkv", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if selected.ID != 2 {
		t.Fatalf("selected ID = %d", selected.ID)
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

func TestQueueReturnsTorrentID(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents/createtorrent" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"torrent_id":42}}`))
	}))
	defer server.Close()

	id, err := (Client{BaseURL: server.URL, Token: "token", HTTP: server.Client()}).Queue(context.Background(), hash)
	if err != nil {
		t.Fatal(err)
	}
	if id != 42 {
		t.Fatalf("id = %d", id)
	}
}

func TestWaitDownloadedPollsUntilFinished(t *testing.T) {
	polls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/torrents/mylist" {
			t.Errorf("path = %q", r.URL.Path)
		}
		polls++
		progress := 0.5
		if polls >= 3 {
			progress = 1
		}
		_, _ = w.Write([]byte(`{"success":true,"data":{"id":42,"progress":` + strconv.FormatFloat(progress, 'f', -1, 64) + `}}`))
	}))
	defer server.Close()

	var progress []float64
	err := (Client{BaseURL: server.URL, Token: "token", HTTP: server.Client(), PollInterval: time.Millisecond}).WaitDownloaded(context.Background(), 42, func(p float64) {
		progress = append(progress, p)
	})
	if err != nil {
		t.Fatal(err)
	}
	if polls < 3 || len(progress) == 0 {
		t.Fatalf("polls = %d, progress = %#v", polls, progress)
	}
}

func TestWaitDownloadedFailsAfterConsecutiveErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	err := (Client{BaseURL: server.URL, Token: "token", HTTP: server.Client(), PollInterval: time.Millisecond}).WaitDownloaded(context.Background(), 42, nil)
	if err == nil {
		t.Fatal("expected error after repeated failures")
	}
}

func TestFindMatchesHashInMyList(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef01234567"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"success":true,"data":[{"id":9,"hash":"other"},{"id":7,"hash":"` + hash + `"}]}`))
	}))
	defer server.Close()

	id, err := (Client{BaseURL: server.URL, Token: "token", HTTP: server.Client()}).Find(context.Background(), strings.ToUpper(hash))
	if err != nil {
		t.Fatal(err)
	}
	if id != 7 {
		t.Fatalf("id = %d", id)
	}
}
