package stremio

import (
	"slices"
	"testing"
)

func TestReleaseLanguageHints(t *testing.T) {
	for _, tc := range []struct {
		text               string
		audio, subs, hints []string
	}{
		{text: "Show.1080p\n🇬🇧 / 🇯🇵", hints: []string{"en", "ja"}},
		{text: "Show.1080p\nAudio: jpn, eng\nSubs: French", audio: []string{"ja", "en"}, subs: []string{"fr"}},
		{text: "Audio: ja / en | Subtitles: de", audio: []string{"ja", "en"}, subs: []string{"de"}},
		{text: "Show.MULTI.Dual.Audio\nMulti Subs\n🇮🇳"},
		{text: "It.Is.Us.The.Movie.1080p.mkv"},
		{text: "Show.ENG.JPN.1080p.mkv", hints: []string{"en", "ja"}},
		{text: "Show\nSubtitles: English", subs: []string{"en"}},
	} {
		t.Run(tc.text, func(t *testing.T) {
			audio, subs, hints := releaseLanguages(tc.text)
			if !slices.Equal(audio, tc.audio) || !slices.Equal(subs, tc.subs) || !slices.Equal(hints, tc.hints) {
				t.Fatalf("audio=%v subs=%v hints=%v", audio, subs, hints)
			}
		})
	}
}
