package metadata

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRetainsUnknownNestedFieldsWithoutCredentials(t *testing.T) {
	fields := Parse([]byte(`{"cast":["Actor A","Actor B"],"futureField":{"enabled":false,"count":0},"files":[{"id":9007199254740993,"mimetype":"video/x-matroska","metadata":{"codec":"hevc","audio":[{"language":"jpn","channels":6}],"access_token":"private-token"},"download_url":"https://example.invalid/private-url"}],"headers":{"Authorization":"private-header"},"poster":"https://example.invalid/private-image","description":"See https://example.invalid/private-link for details","videos":[{"id":"child"}]}`), "videos")
	raw, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"private-", "Authorization", "access_token", "download_url", "videos"} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("retained %q: %s", secret, raw)
		}
	}
	text := strings.Join(Lines("Provider", fields), "\n")
	for _, expected := range []string{"Cast: Actor A, Actor B", "Future Field > Enabled: false", "Future Field > Count: 0", "Files [1] > Id: 9007199254740993", "Files [1] > Metadata > Audio [1] > Channels: 6", "Codec: hevc", "Language: jpn"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q: %s", expected, text)
		}
	}
	for range 5 {
		if strings.Join(Lines("Provider", fields), "\n") != text {
			t.Fatal("unstable field order")
		}
	}
}

func TestEmptyAndMalformedMetadata(t *testing.T) {
	for _, raw := range []string{"null", "false", "[]", "bad JSON", `{}`, `{"empty":[],"missing":null}`} {
		if lines := Lines("Provider", Parse([]byte(raw))); len(lines) != 0 {
			t.Fatalf("%s: %v", raw, lines)
		}
	}
}
