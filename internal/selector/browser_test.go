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
