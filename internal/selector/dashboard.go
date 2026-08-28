package selector

import (
	"context"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type DashboardAction int

const (
	DashboardQuit DashboardAction = iota
	DashboardSearch
	DashboardHistory
)

type DashboardResult struct {
	Action DashboardAction
	Query  string
	Group  string
}

type DashboardOptions struct {
	Groups         []string
	PreferredGroup string
}

type dashboardModel struct {
	query      string
	result     DashboardResult
	width      int
	height     int
	groups     []string
	groupIndex int
}

func (m dashboardModel) Init() tea.Cmd { return nil }

func (m dashboardModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "esc":
			return m, tea.Quit
		case "ctrl+h":
			m.result.Action = DashboardHistory
			return m, tea.Quit
		case "tab":
			if len(m.groups) > 1 {
				m.groupIndex = (m.groupIndex + 1) % len(m.groups)
			}
		case "enter":
			query := strings.TrimSpace(m.query)
			if query != "" {
				m.result = DashboardResult{Action: DashboardSearch, Query: query, Group: m.selectedGroup()}
				return m, tea.Quit
			}
		case "backspace":
			if len(m.query) > 0 {
				runes := []rune(m.query)
				m.query = string(runes[:len(runes)-1])
			}
		case "ctrl+w":
			m.query = strings.TrimRight(m.query, " ")
			if end := strings.LastIndex(m.query, " "); end >= 0 {
				m.query = strings.TrimRight(m.query[:end+1], " ")
			} else {
				m.query = ""
			}
		case "ctrl+u":
			m.query = ""
		default:
			if msg.Type == tea.KeySpace {
				m.query += " "
			} else if msg.Type == tea.KeyRunes {
				m.query += string(msg.Runes)
			}
		}
	}
	return m, nil
}

func (m dashboardModel) View() string {
	tabs := m.mediaTabs()
	content := lipgloss.JoinVertical(lipgloss.Left,
		headerStyle.Render("Lemmewatch"),
		"",
		"What would you like to watch?",
		activeBorder.Width(54).Padding(0, 1).Render(m.query+"_"),
		tabs,
		"",
		hintStyle.Render("Tab movie/series   Enter search   Ctrl-H history   Esc quit"),
	)
	width, height := m.width, m.height
	if width <= 0 {
		width = 80
	}
	if height <= 0 {
		height = 20
	}
	return "\x1b]0;Lemmewatch\x07" + lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, content)
}

func (m dashboardModel) mediaTabs() string {
	return lipgloss.NewStyle().PaddingLeft(1).Render(groupTabs(m.groups, m.groupIndex))
}

func (m dashboardModel) selectedGroup() string {
	if m.groupIndex >= 0 && m.groupIndex < len(m.groups) {
		return m.groups[m.groupIndex]
	}
	return ""
}

func Dashboard(ctx context.Context, input io.Reader, output io.Writer, options DashboardOptions) (DashboardResult, error) {
	initial := dashboardModel{groups: options.Groups, groupIndex: preferredGroupIndex(options.Groups, options.PreferredGroup)}
	program := tea.NewProgram(initial, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output))
	final, err := program.Run()
	if err != nil {
		return DashboardResult{}, err
	}
	return final.(dashboardModel).result, nil
}
