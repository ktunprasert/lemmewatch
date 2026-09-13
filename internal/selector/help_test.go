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

func TestBottomHelpCombinesNavigationAndOmitsEnter(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	bindings := m.shortHelp(browserKeys())
	foundNavigation := false
	for _, binding := range bindings {
		help := binding.Help()
		if help.Key == "hjkl" && help.Desc == "move" {
			foundNavigation = true
		}
		if help.Key == "enter" {
			t.Fatal("bottom help contains Enter hint")
		}
	}
	if !foundNavigation {
		t.Fatal("bottom help missing combined hjkl navigation hint")
	}
}

func TestHelpLinePreservesFullVersionWhenNarrow(t *testing.T) {
	line := renderHelpLine(newHelpModel(), 4, []key.Binding{
		hintBinding("q", "quit"),
	}, helpLineOptions{Right: "v2026.9.4", RightColumn: true})
	if got := ansi.Strip(line); got != "v2026.9.4" {
		t.Fatalf("narrow help line = %q", got)
	}
}

func TestHelpLineCanDisableRightColumn(t *testing.T) {
	line := renderHelpLine(newHelpModel(), 40, []key.Binding{
		hintBinding("q", "quit"),
	}, helpLineOptions{Right: "v2026.9.4"})
	if strings.Contains(ansi.Strip(line), "v2026.9.4") {
		t.Fatalf("disabled right column rendered: %q", line)
	}
}

func TestHelpRowShowsRefreshOnlyWhenAvailable(t *testing.T) {
	refreshable := newBrowser(testChoice{label: "Dune", cacheKey: "streams:tt1"})
	plain := ansi.Strip(refreshable.help.ShortHelpView(refreshable.shortHelp(browserKeys())))
	if !strings.Contains(plain, "r/F5 refresh") {
		t.Fatalf("refresh hint missing: %q", plain)
	}

	unrefreshable := newBrowser(testChoice{label: "Season 1"})
	plain = ansi.Strip(unrefreshable.help.ShortHelpView(unrefreshable.shortHelp(browserKeys())))
	if strings.Contains(plain, "r/F5 refresh") {
		t.Fatalf("refresh hint shown without refresh action: %q", plain)
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
	plain := ansi.Strip(view)
	for _, expected := range []string{"enter search", "esc cancel", "ctrl-w word", "ctrl-u clear"} {
		if !strings.Contains(plain, expected) {
			t.Fatalf("search help missing %q: %q", expected, plain)
		}
	}
}

func TestStreamHelpKeepsHelpAndDirectQualityVisible(t *testing.T) {
	m := newBrowser(testChoice{label: "Movie"})
	m.width = 64
	m.right = pane[testChoice]{title: "Streams", items: []testChoice{{label: "Direct", terminal: true, direct: true, playable: true}}}
	m.focusRight = true
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "? help") || !strings.Contains(view, "v quality") || strings.Contains(view, "c cached/all") {
		t.Fatalf("direct stream hints = %q", view)
	}
}
