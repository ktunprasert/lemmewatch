package selector

import (
	"errors"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestCustomPaneSizesAndInvalidPreferences(t *testing.T) {
	panes := []visiblePane[testChoice]{{title: "Media"}, {title: "Seasons"}, {title: "Episodes", active: true}}
	sizes := map[int][]int{2: {1, 3}, 3: {1, 2, 3}}
	_, widths := paneLayout(120, panes[:2], sizes)
	if !slices.Equal(widths, []int{28, 88}) {
		t.Fatalf("two-pane sizes = %v", widths)
	}
	_, widths = paneLayout(120, panes, sizes)
	if !slices.Equal(widths, []int{18, 38, 58}) {
		t.Fatalf("three-pane sizes = %v", widths)
	}
	for _, invalid := range [][]int{nil, {1}, {1, 2}, {0, 2, 3}, {-1, 2, 3}, {1001, 2, 3}} {
		sizes[3] = invalid
		_, widths = paneLayout(120, panes, sizes)
		if !slices.Equal(widths, []int{22, 46, 46}) {
			t.Fatalf("invalid sizes %v did not use defaults: %v", invalid, widths)
		}
	}
	sizes[3] = []int{1, 1000, 1}
	for _, width := range []int{88, 120, 201} {
		_, widths = paneLayout(width, panes, sizes)
		if paneWidth(widths) != width || slices.Min(widths) < 16 {
			t.Fatalf("extreme sizes exceed viewport or minimum: %v", widths)
		}
	}
}

func TestPaneSizeEditorValidatesSavesAndReturnsToSettings(t *testing.T) {
	m := newBrowser(testChoice{label: "Movie"})
	m.overlay, m.settingsIndex = overlaySettings, 10
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if m.overlay != overlayPaneSizes || m.paneSizeCount != 3 || m.paneSizeValue != "20:40:40" {
		t.Fatalf("size editor = %#v", m)
	}
	saved := false
	m.options.SavePaneSizes = func(count int, sizes []int) error {
		saved = true
		if count != 3 || !slices.Equal(sizes, []int{1, 3, 2}) {
			t.Fatalf("saved %d-pane sizes = %v", count, sizes)
		}
		return nil
	}
	m.paneSizeValue = "0:40:60"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if saved || m.overlay != overlayPaneSizes || m.toastText() == "" {
		t.Fatal("invalid sizes accepted or error missing")
	}
	m.paneSizeValue = "1:3:2"
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !saved || m.overlay != overlaySettings || !slices.Equal(m.paneSizes[3], []int{1, 3, 2}) || m.toastText() != "" {
		t.Fatal("sizes not applied or settings not restored")
	}
	if !strings.Contains(m.settingsModal(), "1:3:2") {
		t.Fatal("settings do not show updated ratios")
	}
}

func TestCustomNarrowPaneContainsErrorAndRetryText(t *testing.T) {
	m := newBrowser(testChoice{label: "Movie"})
	m.width, m.height = 88, 24
	m.paneSizes = map[int][]int{2: {1000, 1}}
	m.right.title = "Streams"
	m.err = errors.New("A long failure\nwith another line")
	lines := strings.Split(strings.TrimSuffix(ansi.Strip(m.View()), "\n"), "\n")
	if len(lines) != m.height {
		t.Fatalf("error expanded narrow pane: %d lines", len(lines))
	}
	for _, line := range lines {
		if lipgloss.Width(line) > m.width {
			t.Fatalf("error overflowed terminal: %q", line)
		}
	}
}

func TestPaneSizeEditorPreservesSizesOnCancelAndSaveFailure(t *testing.T) {
	m := newBrowser(testChoice{label: "Movie"})
	m.paneSizes = map[int][]int{2: {1, 1}}
	m.overlay, m.settingsIndex = overlaySettings, 9
	m.changeSetting(1)
	m.paneSizeValue = "1:3"
	m.options.SavePaneSizes = func(int, []int) error { return errors.New("disk full") }
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !slices.Equal(m.paneSizes[2], []int{1, 1}) || m.overlay != overlayPaneSizes || m.toastText() == "" {
		t.Fatal("save failure changed layout or closed editor")
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(browserModel[testChoice])
	if m.overlay != overlaySettings || !slices.Equal(m.paneSizes[2], []int{1, 1}) {
		t.Fatal("cancel changed layout or lost settings")
	}
}
