package selector

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/x/ansi"
)

func TestHelpRowHighlightsKeysAndShowsVersion(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.width = 80
	m.options.Version = "v2026.9.4"
	view := ansi.Strip(m.View())
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	last := lines[len(lines)-1]
	if !strings.HasSuffix(last, "v2026.9.4") {
		t.Fatalf("version not right-aligned: %q", last)
	}
	if !strings.Contains(m.View(), versionStyle.Render("v2026.9.4")) {
		t.Fatalf("version not accent underlined: %q", m.View())
	}
	styled := m.help.ShortHelpView(m.shortHelp(browserKeys()))
	if !strings.Contains(styled, headerStyle.Render("q")) {
		t.Fatalf("key not accent styled: %q", styled)
	}
	if !strings.Contains(styled, hintStyle.Render("quit")) {
		t.Fatalf("description not faint: %q", styled)
	}
}

func TestHelpLinePreservesFullVersionWhenNarrow(t *testing.T) {
	line := renderHelpLine(newHelpModel(), 4, []key.Binding{
		hintBinding("q", "quit"),
	}, "v2026.9.4")
	if got := ansi.Strip(line); got != "v2026.9.4" {
		t.Fatalf("narrow help line = %q", got)
	}
}

func TestSearchOverlayUsesSharedHelpStyles(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.width = 100
	m.overlay = overlayQuery
	view := m.View()
	if !strings.Contains(view, headerStyle.Render("enter")) {
		t.Fatalf("overlay key not accent styled: %q", view)
	}
	if !strings.Contains(view, hintStyle.Render("search")) {
		t.Fatalf("overlay description not faint: %q", view)
	}
}
