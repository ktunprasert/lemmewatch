package selector

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"lemmewatch/internal/model"
)

func TestSettingsEditOrderedLanguagesAndSpeed(t *testing.T) {
	for _, tc := range []struct {
		index int
		value string
	}{{11, "Japanese,en"}, {12, "eng,fr"}, {13, "1.35"}} {
		m := newBrowser(testChoice{label: "Show"})
		m.overlay, m.settingsIndex = overlaySettings, tc.index
		var saved model.PlaybackPreferences
		m.options.SavePlayback = func(p model.PlaybackPreferences) error { saved = p; return nil }
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(browserModel[testChoice])
		if m.overlay != overlayPlaybackSetting {
			t.Fatal("missing editor")
		}
		m.playbackSettingValue = tc.value
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(browserModel[testChoice])
		if m.overlay != overlaySettings {
			t.Fatal("did not return to settings")
		}
		switch tc.index {
		case 11:
			if !slices.Equal(saved.AudioLanguages, []string{"ja", "en"}) {
				t.Fatal(saved)
			}
		case 12:
			if !slices.Equal(saved.SubtitleLanguages, []string{"en", "fr"}) {
				t.Fatal(saved)
			}
		case 13:
			if saved.PlaybackSpeed != 1.35 {
				t.Fatal(saved)
			}
		}
		m.openOverlay(overlayPlaybackSetting)
		m.playbackSettingValue = ""
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(browserModel[testChoice])
		if len(saved.AudioLanguages)+len(saved.SubtitleLanguages) != 0 || saved.PlaybackSpeed != 0 {
			t.Fatal("clear did not reset default")
		}
	}
}

func TestPlaybackSettingsRejectInvalidOrFailedSave(t *testing.T) {
	for _, tc := range []struct {
		index    int
		value    string
		failSave bool
	}{
		{11, "en,,ja", false}, {13, "NaN", false}, {13, "5", false}, {13, "0", false}, {11, "en", true},
	} {
		m := newBrowser(testChoice{label: "Show"})
		m.overlay, m.settingsIndex, m.playbackSettingValue = overlayPlaybackSetting, tc.index, tc.value
		m.options.SavePlayback = func(model.PlaybackPreferences) error {
			if !tc.failSave {
				t.Fatal("invalid value saved")
			}
			return errors.New("disk full")
		}
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		m = next.(browserModel[testChoice])
		if m.overlay != overlayPlaybackSetting || len(m.playbackPreferences.AudioLanguages) != 0 || m.playbackPreferences.PlaybackSpeed != 0 {
			t.Fatal("failed setting applied")
		}
	}
}

func TestSmallSettingsModalKeepsPlaybackSelectionVisible(t *testing.T) {
	m := newBrowser(testChoice{label: "Show"})
	m.height, m.settingsIndex = 12, 13
	view := ansi.Strip(m.settingsModal())
	if !strings.Contains(view, "> Playback speed") || len(strings.Split(view, "\n")) > m.height {
		t.Fatalf("settings overflow: %q", view)
	}
}

func TestAutoplaySettingPersists(t *testing.T) {
	m := newBrowser(testChoice{label: "Show"})
	m.settingsIndex = 14
	m.overlay = overlaySettings
	saved := false
	m.options.SaveAutoplay = func(value bool) error { saved = value; return nil }
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(browserModel[testChoice])
	if !m.autoplay || !saved || !strings.Contains(ansi.Strip(m.settingsModal()), "Autoplay next episode") {
		t.Fatalf("autoplay setting = %t, saved = %t", m.autoplay, saved)
	}
}

func TestStreamLanguageRankingAndFallback(t *testing.T) {
	m := newBrowser(testChoice{label: "Show"})
	m.cachedOnly = false
	m.right.items = []testChoice{
		{label: "known other", terminal: true, audioLanguages: []string{"de"}},
		{label: "unknown", terminal: true},
		{label: "fallback", terminal: true, audioLanguages: []string{"en"}},
		{label: "preferred audio", terminal: true, audioLanguages: []string{"ja"}},
		{label: "both preferred", terminal: true, audioLanguages: []string{"ja"}, subtitleLanguages: []string{"en"}},
		{label: "wrong episode", terminal: true, audioLanguages: []string{"ja"}, subtitleLanguages: []string{"en"}, matchRank: 2},
	}
	m.playbackPreferences = model.PlaybackPreferences{AudioLanguages: []string{"ja", "en"}, SubtitleLanguages: []string{"en"}}
	var labels []string
	for _, entry := range m.filteredRight() {
		labels = append(labels, entry.item.label)
	}
	want := []string{"both preferred", "preferred audio", "fallback", "unknown", "known other", "wrong episode"}
	if !slices.Equal(labels, want) {
		t.Fatalf("rank = %v", labels)
	}
	m.playbackPreferences = model.PlaybackPreferences{}
	if m.filteredRight()[0].item.label != "known other" {
		t.Fatal("unset language changed base ranking")
	}
	// Ambiguous release flags may help audio ranking, but do not claim subtitles.
	m.right.items = []testChoice{{label: "unknown", terminal: true}, {label: "hint", terminal: true, languageHints: []string{"ja"}}}
	m.playbackPreferences.AudioLanguages = []string{"ja"}
	if m.filteredRight()[0].item.label != "hint" {
		t.Fatal("ignored release hint")
	}
	m.playbackPreferences = model.PlaybackPreferences{SubtitleLanguages: []string{"ja"}}
	if m.filteredRight()[0].item.label != "unknown" {
		t.Fatal("audio hint claimed subtitles")
	}
}
