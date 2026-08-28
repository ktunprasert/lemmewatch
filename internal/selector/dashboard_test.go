package selector

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestDashboardSearchesEnteredQuery(t *testing.T) {
	m := dashboardModel{groups: []string{"movie", "series"}}
	for _, message := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("Monogatari")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("Series")},
		{Type: tea.KeyEnter},
	} {
		next, _ := m.Update(message)
		m = next.(dashboardModel)
	}
	if m.result.Action != DashboardSearch || m.result.Query != "Monogatari Series" || m.result.Group != "movie" {
		t.Fatalf("result = %#v", m.result)
	}
}

func TestDashboardTabsMediaGroup(t *testing.T) {
	m := dashboardModel{groups: []string{"movie", "series"}}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(dashboardModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Dune")})
	m = next.(dashboardModel)
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(dashboardModel)
	if m.result.Group != "series" {
		t.Fatalf("result = %#v", m.result)
	}
}

func TestDashboardOpensHistory(t *testing.T) {
	m := dashboardModel{}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	m = next.(dashboardModel)
	if m.result.Action != DashboardHistory {
		t.Fatalf("result = %#v", m.result)
	}
}

func TestDashboardShowsPromptAndHints(t *testing.T) {
	view := ansi.Strip((dashboardModel{width: 80, height: 20, groups: []string{"movie", "series"}}).View())
	for _, expected := range []string{"What would you like to watch?", "● Movies", "○ Series", "Tab movie/series", "Enter search", "Ctrl-H history", "Esc quit"} {
		if !strings.Contains(view, expected) {
			t.Fatalf("missing %q in %q", expected, view)
		}
	}
}
