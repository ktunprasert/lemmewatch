package selector

import (
	"context"
	"errors"
	"fmt"
	"io"

	tea "github.com/charmbracelet/bubbletea"
)

var ErrCancelled = errors.New("selection cancelled")

type item interface{ Label() string }

type model[T item] struct {
	items  []T
	index  int
	height int
	chosen bool
	quit   bool
}

func (m model[T]) Init() tea.Cmd { return nil }
func (m model[T]) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = max(1, msg.Height-2)
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q", "esc":
			m.quit = true
			return m, tea.Quit
		case "enter":
			m.chosen = true
			return m, tea.Quit
		case "up", "k":
			if m.index > 0 {
				m.index--
			}
		case "down", "j":
			if m.index+1 < len(m.items) {
				m.index++
			}
		case "pgup":
			m.index = max(0, m.index-max(1, m.height))
		case "pgdown":
			m.index = min(len(m.items)-1, m.index+max(1, m.height))
		}
	}
	return m, nil
}
func (m model[T]) View() string {
	if len(m.items) == 0 {
		return "No choices\n"
	}
	height := m.height
	if height <= 0 {
		height = 10
	}
	start := max(0, min(m.index-height/2, len(m.items)-height))
	end := min(len(m.items), start+height)
	s := ""
	for i := start; i < end; i++ {
		cursor := "  "
		if i == m.index {
			cursor = "> "
		}
		s += fmt.Sprintf("%s%s\n", cursor, m.items[i].Label())
	}
	return s
}

func Choose[T item](ctx context.Context, input io.Reader, output io.Writer, items []T) (T, error) {
	var zero T
	if len(items) == 0 {
		return zero, errors.New("no choices")
	}
	initial := model[T]{items: items, height: 10}
	p := tea.NewProgram(initial, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output))
	final, err := p.Run()
	if err != nil {
		return zero, err
	}
	m := final.(model[T])
	if !m.chosen || m.quit {
		return zero, ErrCancelled
	}
	return m.items[m.index], nil
}
