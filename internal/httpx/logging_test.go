package httpx

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLoggingOmitsQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()

	verbose := true
	var log bytes.Buffer
	client := &http.Client{Transport: LoggingTransport{Verbose: &verbose, Output: &log}}
	res, err := client.Get(server.URL + "/requestdl?token=very-secret")
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if strings.Contains(log.String(), "very-secret") || strings.Contains(log.String(), "token=") {
		t.Fatalf("query leaked: %q", log.String())
	}
	if !strings.Contains(log.String(), "/requestdl -> 204") {
		t.Fatalf("missing request details: %q", log.String())
	}
}
