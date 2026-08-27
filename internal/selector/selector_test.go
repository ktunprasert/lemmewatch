package selector

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
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
