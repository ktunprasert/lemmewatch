package selector

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestPopupTitlesLiveInBorders(t *testing.T) {
	m := newBrowser(testChoice{label: "Movie"})
	m.height = 24
	for _, tt := range []struct {
		title, view, firstRow string
	}{
		{"Settings", m.settingsModal(), "Media type"},
		{"Keybindings", m.helpModal(), "Search:"},
		{"Search", inputModal("Search", "Dune", 50, nil), "Dune_"},
		{"Filter active pane", inputModal("Filter active pane", "1080", 50, nil), "1080_"},
		{"Custom player", inputModal("Custom player", "mpv", 50, nil), "mpv_"},
		{"TorBox API key", inputModal("TorBox API key", "***", 50, nil), "***_"},
		{"3-pane sizes", inputModal("3-pane sizes", "1:2:3", 50, nil), "1:2:3_"},
		{"Sort results", sortModal(false, false), "a   Name ascending"},
		{"Sort streams", sortModal(true, false), "q   Quality ascending"},
		{"Mode", modeModal([]ContextMode{{Key: "i", Name: "ID"}}), "[i] ID"},
	} {
		t.Run(tt.title, func(t *testing.T) {
			lines := strings.Split(ansi.Strip(tt.view), "\n")
			if !strings.HasPrefix(lines[0], "╭─ "+tt.title) || !strings.HasSuffix(lines[0], "╮") {
				t.Fatalf("title not in border: %q", lines[0])
			}
			if !strings.Contains(lines[1], tt.firstRow) {
				t.Fatalf("first content row wasted: %q", lines[1])
			}
			width := lipgloss.Width(lines[0])
			for _, line := range lines {
				if lipgloss.Width(line) != width {
					t.Fatalf("border/content widths differ: %q", line)
				}
			}
		})
	}
}

func TestInputModalKeepsLongTextOnOneRow(t *testing.T) {
	view := ansi.Strip(inputModal("Search", strings.Repeat("界", 100), 30, nil))
	lines := strings.Split(view, "\n")
	if len(lines) != 5 || !strings.Contains(lines[1], "…_") {
		t.Fatalf("long input wrapped or cursor lost: %q", view)
	}
	for _, line := range lines {
		if lipgloss.Width(line) != 32 {
			t.Fatalf("input modal overflowed: %q", line)
		}
	}
}
