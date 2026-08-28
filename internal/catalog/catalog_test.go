package catalog

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSearch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/catalog/movie/top/search=Dune.json") {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"metas":[{"id":"tt1160419","type":"movie","name":"Dune","releaseInfo":"2021","imdbRating":"8.0"}]}`))
	}))
	defer server.Close()
	items, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Search(context.Background(), "movie", "Dune")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "tt1160419" || items[0].Year != 2021 || items[0].Rating != "8.0" {
		t.Fatalf("items = %#v", items)
	}
}

func TestSearchRejectsHTTPErrorWithoutBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "secret", http.StatusBadGateway) }))
	defer server.Close()
	_, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Search(context.Background(), "movie", "Dune")
	if err == nil || strings.Contains(err.Error(), "secret") {
		t.Fatalf("error = %v", err)
	}
}

func TestEpisodesFiltersSpecials(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/meta/series/tt123.json" {
			t.Errorf("path = %q", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"meta":{"videos":[{"id":"tt123:0:1","name":"Special","season":0,"episode":1},{"id":"tt123:1:1","name":"Pilot","season":1,"episode":1,"released":"2026-08-28T00:00:00Z","rating":"8.4"},{"id":"tt123:1:2","title":"Legacy Title","season":1,"episode":2,"rating":"0"}]}}`))
	}))
	defer server.Close()
	episodes, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Episodes(context.Background(), "tt123")
	if err != nil {
		t.Fatal(err)
	}
	if len(episodes) != 2 || episodes[0].ID != "tt123:1:1" || episodes[0].Title != "Pilot" || episodes[1].Title != "Legacy Title" {
		t.Fatalf("episodes = %#v", episodes)
	}
	if got := episodes[0].Released.Format("2006-01-02"); got != "2026-08-28" {
		t.Fatalf("released = %q", got)
	}
	if episodes[0].Rating != "8.4" || episodes[1].Rating != "" {
		t.Fatalf("ratings = %q, %q", episodes[0].Rating, episodes[1].Rating)
	}
}
