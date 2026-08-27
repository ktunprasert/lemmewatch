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
		_, _ = w.Write([]byte(`{"metas":[{"id":"tt1160419","type":"movie","name":"Dune","releaseInfo":"2021"}]}`))
	}))
	defer server.Close()
	items, err := (Client{BaseURL: server.URL, HTTP: server.Client()}).Search(context.Background(), "movie", "Dune")
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "tt1160419" || items[0].Year != 2021 {
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
