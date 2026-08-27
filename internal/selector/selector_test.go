package selector

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type choice string

func (c choice) Label() string { return string(c) }

func TestKeyboardNavigation(t *testing.T) {
	m := model[choice]{items: []choice{"a", "b", "c"}, height: 1}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	m = next.(model[choice])
	if m.index != 1 {
		t.Fatalf("index = %d", m.index)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(model[choice])
	if !m.chosen {
		t.Fatal("enter did not choose")
	}
}

func TestViewShowsKeyHints(t *testing.T) {
	view := (model[choice]{items: []choice{"a"}, height: 1}).View()
	if !strings.Contains(view, "enter select") || !strings.Contains(view, "esc cancel") {
		t.Fatalf("missing key hints: %q", view)
	}
}

func TestViewHighlightsAndTruncatesSelection(t *testing.T) {
	view := (model[choice]{items: []choice{"a very long selection label that must not wrap"}, height: 1, width: 24}).View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "> a very long selec...") {
		t.Fatalf("selection not visible or truncated: %q", plain)
	}
}

func TestViewFlattensMultilineLabelsAndShowsRange(t *testing.T) {
	view := (model[choice]{items: []choice{"first\nline", "second"}, height: 1, width: 40}).View()
	plain := ansi.Strip(view)
	if !strings.Contains(plain, "Select an option  1-1/2") {
		t.Fatalf("missing visible range: %q", plain)
	}
	if !strings.Contains(plain, "> first line") {
		t.Fatalf("multiline label not flattened: %q", plain)
	}
}
