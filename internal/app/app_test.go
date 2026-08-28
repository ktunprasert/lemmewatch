package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/config"
	"lemmewatch/internal/model"
)

func TestSearchPreservesCatalogRelevanceWithinMediaType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/movie/"):
			_, _ = w.Write([]byte(`{"metas":[{"id":"tt2","type":"movie","name":"Zulu"},{"id":"tt1","type":"movie","name":"Alpha"}]}`))
		case strings.Contains(r.URL.Path, "/series/"):
			_, _ = w.Write([]byte(`{"metas":[{"id":"tt4","type":"series","name":"Yellow"},{"id":"tt3","type":"series","name":"Beta"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	a := App{Catalog: catalog.Client{BaseURL: server.URL, HTTP: server.Client()}, Err: io.Discard}
	items, err := a.Search(context.Background(), "query", "")
	if err != nil {
		t.Fatal(err)
	}
	var movies, series []string
	for _, item := range items {
		switch item.Type {
		case model.Movie:
			movies = append(movies, item.Name)
		case model.Series:
			series = append(series, item.Name)
		}
	}
	if strings.Join(movies, ",") != "Zulu,Alpha" {
		t.Fatalf("movie order = %#v", movies)
	}
	if strings.Join(series, ",") != "Yellow,Beta" {
		t.Fatalf("series order = %#v", series)
	}
}

func TestHistoryMediaPreservesPlayableEntries(t *testing.T) {
	items := historyMedia([]config.HistoryEntry{
		{ID: "tt1", Title: "Movie", Type: "movie"},
		{ID: "bad", Title: "Invalid", Type: "podcast"},
		{ID: "tt2", Title: "Series", Type: "series"},
	})
	if len(items) != 2 || items[0].Name != "Movie" || items[1].Type != model.Series {
		t.Fatalf("items = %#v", items)
	}
}
