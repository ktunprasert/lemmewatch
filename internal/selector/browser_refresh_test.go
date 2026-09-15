package selector

import (
	"context"
	"errors"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestMetadataRefreshFromSeasonAndEpisodePanes(t *testing.T) {
	for _, depth := range []int{1, 2} {
		for _, focusRight := range []bool{false, true} {
			for _, fail := range []bool{false, true} {
				m := newBrowser(testChoice{label: "Show", cacheKey: "series:show"})
				seasons := pane[testChoice]{title: "Seasons", items: []testChoice{{label: "Season 1", cacheKey: "season:1"}}}
				episodes := pane[testChoice]{title: "Episodes", items: []testChoice{{label: "Old title", cacheKey: "episode:1"}}}
				m.crumbs = []string{"Show"}
				m.right = seasons
				if depth == 2 {
					m.levels = append(m.levels, seasons)
					m.right = episodes
					m.crumbs = append(m.crumbs, "Season 1")
				}
				if !focusRight {
					m.levels = append(m.levels, m.right)
					m.right = pane[testChoice]{}
				}
				m.focusRight = focusRight
				m.options.ChildTitle = func(c testChoice) string {
					if c.MetadataRoot() {
						return "Seasons"
					}
					return "Episodes"
				}
				m.options.Refresh = func(_ context.Context, c testChoice) ([]testChoice, error) {
					if !c.MetadataRoot() {
						t.Fatalf("refreshed %q instead of show", c.label)
					}
					if fail {
						return nil, errors.New("offline")
					}
					return append(seasons.items, testChoice{label: "Season 2"}), nil
				}
				m.load = func(_ context.Context, c testChoice) ([]testChoice, error) {
					if c.cacheKey != "season:1" {
						t.Fatalf("loaded %q", c.label)
					}
					return []testChoice{{label: "Updated title", cacheKey: "episode:1"}, {label: "New episode"}}, nil
				}
				beforeLevels, beforeRight := len(m.levels), m.right.title
				next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF5})
				if cmd == nil {
					t.Fatalf("no refresh: depth=%d right=%v", depth, focusRight)
				}
				m = next.(browserModel[testChoice])
				next, _ = m.Update(cmd())
				m = next.(browserModel[testChoice])
				if fail {
					if len(m.levels) != beforeLevels || m.right.title != beforeRight {
						t.Fatal("failure replaced good panes")
					}
				} else if len(m.right.items) != 2 || len(m.levels) != depth || !m.focusRight {
					t.Fatalf("refresh lost path: %#v", m)
				} else if depth == 2 && m.right.items[0].label != "Updated title" {
					t.Fatal("stale episodes")
				}
			}
		}
	}
}

func TestMetadataRefreshUsesSortedRootSelection(t *testing.T) {
	m := newBrowser(testChoice{label: "Zebra", cacheKey: "series:other"}, testChoice{label: "Alpha", cacheKey: "series:show"})
	m.sortMode = sortNameAscending
	m.levels = append(m.levels, pane[testChoice]{title: "Seasons"})
	m.crumbs = []string{"Alpha"}
	m.options.Refresh = func(_ context.Context, selected testChoice) ([]testChoice, error) {
		if selected.label != "Alpha" {
			t.Fatalf("wrong show: %q", selected.label)
		}
		return nil, nil
	}
	if !m.canRefresh() {
		t.Fatal("sorted root lost refresh target")
	}
	_, cmd := m.Update(tea.KeyMsg{Type: tea.KeyF5})
	if cmd == nil {
		t.Fatal("missing refresh")
	}
	cmd()
}
