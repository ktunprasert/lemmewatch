package selector

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type choice string

func (c choice) Label() string { return string(c) }

type groupedChoice struct {
	label string
	group string
}

func (c groupedChoice) Label() string { return c.label }
func (c groupedChoice) Group() string { return c.group }

type streamChoiceTest struct {
	label   string
	cached  bool
	quality int
}

func (c streamChoiceTest) Label() string     { return c.label }
func (c streamChoiceTest) IsCached() bool    { return c.cached }
func (c streamChoiceTest) VideoQuality() int { return c.quality }

func TestBrowserLoadsRightPaneAndChoosesChild(t *testing.T) {
	m := browserModel[choice, choice]{
		ctx:     context.Background(),
		parents: []choice{"movie"},
		load: func(_ context.Context, parent choice) ([]choice, error) {
			if parent != "movie" {
				t.Fatalf("parent = %q", parent)
			}
			return []choice{"torrent"}, nil
		},
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[choice, choice])
	if !m.loading || command == nil {
		t.Fatal("enter did not start loading")
	}
	next, _ = m.Update(command())
	m = next.(browserModel[choice, choice])
	if !m.focusRight || len(m.children) != 1 {
		t.Fatalf("right pane not focused after load: %#v", m)
	}
	next, command = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[choice, choice])
	if !m.chosen || m.choice != "torrent" || command == nil {
		t.Fatalf("child not chosen: %#v", m)
	}
}

func TestBrowserFiltersActivePane(t *testing.T) {
	m := browserModel[choice, choice]{parents: []choice{"Dune", "Arrival"}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}})
	m = next.(browserModel[choice, choice])
	for _, key := range []rune("arr") {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = next.(browserModel[choice, choice])
	}
	items := m.filteredParents()
	if len(items) != 1 || items[0].index != 1 || items[0].item != "Arrival" {
		t.Fatalf("filtered items = %#v", items)
	}
	if m.leftIndex != 0 {
		t.Fatalf("filtered selection = %d", m.leftIndex)
	}
}

func TestBrowserShowsLoadingErrorAndBreadcrumb(t *testing.T) {
	m := browserModel[choice, choice]{parents: []choice{"Dune"}, openedLabel: "Dune", loading: true, width: 80, height: 16}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Search / Dune") || !strings.Contains(view, "Loading...") {
		t.Fatalf("loading view = %q", view)
	}
	m.loading = false
	m.err = errors.New("service unavailable")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "Error: service unavailable") || !strings.Contains(view, "Press Enter to retry") {
		t.Fatalf("error view = %q", view)
	}
}

func TestBrowserFlattensRemoteLabels(t *testing.T) {
	m := browserModel[choice, choice]{parents: []choice{"Dune\nPart One\x1b[31m"}, width: 60, height: 12}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Dune Part One") {
		t.Fatalf("label not flattened: %q", view)
	}
	if strings.Contains(view, "\x1b[31m") {
		t.Fatalf("remote escape retained: %q", view)
	}
}

func TestBrowserTabsParentGroups(t *testing.T) {
	m := browserModel[groupedChoice, choice]{
		parents: []groupedChoice{{label: "Dune", group: "movie"}, {label: "Silo", group: "series"}},
		options: BrowserOptions{ParentGroups: []string{"movie", "series"}},
	}
	if got := m.filteredParents(); len(got) != 1 || got[0].item.label != "Dune" {
		t.Fatalf("movie tab = %#v", got)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(browserModel[groupedChoice, choice])
	if got := m.filteredParents(); len(got) != 1 || got[0].item.label != "Silo" {
		t.Fatalf("series tab = %#v", got)
	}
}

func TestBrowserFiltersCacheAndQuality(t *testing.T) {
	m := browserModel[choice, streamChoiceTest]{
		children: []streamChoiceTest{
			{label: "1080p first", cached: true, quality: 1080},
			{label: "2160p second", cached: false, quality: 2160},
		},
		cachedOnly: true,
		quality:    1080,
	}
	if got := m.filteredChildren(); len(got) != 1 || got[0].item.quality != 1080 {
		t.Fatalf("cached quality = %#v", got)
	}
	m.cachedOnly = false
	m.quality = 2160
	if got := m.filteredChildren(); len(got) != 1 || got[0].item.cached {
		t.Fatalf("all quality = %#v", got)
	}
}

func TestFilterEditingShortcuts(t *testing.T) {
	m := browserModel[choice, choice]{parents: []choice{"one two three"}, filtering: true, leftFilter: "one two three"}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(browserModel[choice, choice])
	if m.leftFilter != "one two" {
		t.Fatalf("ctrl-w filter = %q", m.leftFilter)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = next.(browserModel[choice, choice])
	if m.leftFilter != "" {
		t.Fatalf("ctrl-u filter = %q", m.leftFilter)
	}
}

func TestQualityCyclePersists(t *testing.T) {
	saved := -1
	m := browserModel[choice, streamChoiceTest]{focusRight: true, options: BrowserOptions{SaveQuality: func(quality int) error { saved = quality; return nil }}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(browserModel[choice, streamChoiceTest])
	if m.quality != 2160 || saved != 2160 {
		t.Fatalf("quality = %d, saved = %d", m.quality, saved)
	}
}
