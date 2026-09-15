package selector

import (
	"context"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestInfoTogglesSurviveDrillingAndSlidingBack(t *testing.T) {
	m := newBrowser(testChoice{label: "Other"}, testChoice{label: "Show"})
	m.current().index = 1
	m.width, m.height = 120, 24
	m.options.ChildTitle = func(item testChoice) string {
		return map[string]string{"Show": "Seasons", "Season 1": "Episodes", "Episode 1": "Streams"}[item.label]
	}
	m.load = func(_ context.Context, item testChoice) ([]testChoice, error) {
		switch item.label {
		case "Show":
			return []testChoice{{label: "Season 1"}}, nil
		case "Season 1":
			return []testChoice{{label: "Episode 1"}}, nil
		default:
			return []testChoice{{label: "Stream", terminal: true, cached: true}}, nil
		}
	}
	for _, kind := range []string{"media", "season", "episode", "stream"} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
		m = next.(browserModel[testChoice])
		if !m.info[kind].open {
			t.Fatalf("%s info did not open: %#v", kind, m.info)
		}
		if kind != "stream" {
			next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
			m = next.(browserModel[testChoice])
			next, _ = m.Update(runAsync(cmd))
			m = next.(browserModel[testChoice])
		}
	}
	view := ansi.Strip(m.View())
	if strings.Count(view, "├─ Info") != 3 || strings.Contains(strings.Split(view, "\n")[1], "Search") {
		t.Fatalf("three independent info sections or sliding viewport missing:\n%s", view)
	}
	m.back()
	m.back()
	view = ansi.Strip(m.View())
	if m.levels[0].index != 1 || !strings.Contains(strings.Split(view, "\n")[1], "Search") || strings.Count(view, "├─ Info") != 3 {
		t.Fatalf("back lost search selection or info toggles:\n%s", view)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	if m.info["episode"].open || !m.info["media"].open || !m.info["season"].open || !m.info["stream"].open {
		t.Fatalf("toggle affected another pane: %#v", m.info)
	}
}

func TestInfoFollowsSelectionAndFilter(t *testing.T) {
	m := newBrowser(
		testChoice{label: "First", infoLines: []string{"First metadata"}},
		testChoice{label: "Second", infoLines: []string{"Second metadata"}},
	)
	m.toggleInfo()
	if !strings.Contains(ansi.Strip(m.View()), "First metadata") {
		t.Fatal("initial info missing")
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(browserModel[testChoice])
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Second metadata") || strings.Contains(view, "First metadata") {
		t.Fatalf("info did not follow selection: %s", view)
	}
	m.current().filter, m.current().index = "First", 0
	if !strings.Contains(ansi.Strip(m.View()), "First metadata") {
		t.Fatal("info used unfiltered selection")
	}
	m.current().filter = "No match"
	view = ansi.Strip(m.View())
	if strings.Contains(view, "metadata") || !strings.Contains(view, "No item selected") {
		t.Fatalf("empty filter retained stale info: %s", view)
	}
}

func TestInfoScrollAndPagingUseSeparateViewports(t *testing.T) {
	var details []string
	for i := range 30 {
		details = append(details, fmt.Sprintf("Metadata %02d", i))
	}
	items := make([]testChoice, 40)
	for i := range items {
		items[i] = testChoice{label: fmt.Sprintf("Item %02d", i), infoLines: []string{fmt.Sprintf("Details %02d", i)}}
	}
	items[0].infoLines = details
	m := newBrowser(items...)
	m.width, m.height = 64, 24
	m.toggleInfo()
	if m.pageSize() != 14 {
		t.Fatalf("page size includes info section: %d", m.pageSize())
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{'j'}})
	m = next.(browserModel[testChoice])
	view := ansi.Strip(m.View())
	if m.current().index != 0 || strings.Contains(view, "Metadata 00") || !strings.Contains(view, "Metadata 05") {
		t.Fatalf("info scroll moved list or failed: %s", view)
	}
	m.scrollInfo(1000)
	if !strings.Contains(ansi.Strip(m.View()), "Metadata 29") {
		t.Fatal("end of metadata unreachable")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(browserModel[testChoice])
	if m.current().index != 7 || !strings.Contains(ansi.Strip(m.View()), "Details 07") {
		t.Fatalf("half-page or new selection scroll reset failed: %d", m.current().index)
	}
	m.height = 42
	view = ansi.Strip(m.View())
	if len(strings.Split(strings.TrimSuffix(view, "\n"), "\n")) != 42 {
		t.Fatal("resize overflowed viewport")
	}
}

func TestInfoWrapsUnicodeAndKeepsFramesInsideTerminal(t *testing.T) {
	for _, width := range []int{40, 64, 88, 120} {
		for _, height := range []int{8, 24, 42} {
			m := newBrowser(testChoice{label: "Series"})
			m.width, m.height = width, height
			m.levels = append(m.levels, pane[testChoice]{title: "Seasons", items: []testChoice{{label: "Season 1", infoLines: []string{strings.Repeat("界", 100)}}}})
			m.right = pane[testChoice]{title: "Episodes", items: []testChoice{{label: "Episode 1", infoLines: []string{strings.Repeat("👋🏽 words ", 20)}}}}
			m.focusRight = true
			m.info = map[string]paneInfoState{"media": {open: true}, "season": {open: true}, "episode": {open: true}}
			view := ansi.Strip(m.View())
			lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
			if len(lines) != height {
				t.Fatalf("%dx%d: rendered %d lines", width, height, len(lines))
			}
			for _, line := range lines {
				if lipgloss.Width(line) > width {
					t.Fatalf("%dx%d: overflow: %q", width, height, line)
				}
			}
			if !strings.Contains(view, "├─ Info") || strings.Contains(view, "👋") || strings.Contains(view, "🏽") || !strings.Contains(view, "words") {
				t.Fatalf("%dx%d: info missing or emoji retained: %s", width, height, view)
			}
		}
	}
}

func TestInfoHelpRunsToggleAndKeepsModeShortcut(t *testing.T) {
	m := newBrowser(testChoice{label: "Movie", modes: []ContextMode{{Group: "media", Key: "i", Name: "ID", Value: "tt1"}}})
	m.overlay, m.helpFilter = overlayHelp, "Toggle active pane info"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !m.info["media"].open || m.overlay != overlayNone {
		t.Fatal("help action did not toggle info")
	}
	m.overlay = overlayMode
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	if !m.info["media"].open || m.mode["media"] != "i" {
		t.Fatal("info toggle intercepted ID detail mode")
	}
}
