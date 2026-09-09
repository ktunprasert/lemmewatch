package selector

import (
	"strings"
	"testing"

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
	styled := m.help.ShortHelpView(m.shortHelp(browserKeys()))
	if !strings.Contains(styled, headerStyle.Render("q")) {
		t.Fatalf("key not accent styled: %q", styled)
	}
	if !strings.Contains(styled, hintStyle.Render("quit")) {
		t.Fatalf("description not faint: %q", styled)
	}
}
