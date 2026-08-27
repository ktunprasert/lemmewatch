package selector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type Loader[P item, C item] func(context.Context, P) ([]C, error)

type BrowserOptions struct {
	ParentGroups     []string
	PreferredQuality int
	SaveQuality      func(int) error
}

type groupedItem interface{ Group() string }
type streamItem interface {
	IsCached() bool
	VideoQuality() int
}

type indexed[T item] struct {
	index int
	item  T
}

type loaded[T item] struct {
	items []T
	err   error
}

type browserModel[P item, C item] struct {
	ctx         context.Context
	parents     []P
	children    []C
	load        Loader[P, C]
	options     BrowserOptions
	groupIndex  int
	cachedOnly  bool
	quality     int
	leftIndex   int
	rightIndex  int
	focusRight  bool
	loading     bool
	err         error
	filtering   bool
	leftFilter  string
	rightFilter string
	openedLabel string
	notice      string
	width       int
	height      int
	chosen      bool
	choice      C
}

var (
	activeBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"})
	inactiveBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: "#555555"})
)

func (m browserModel[P, C]) Init() tea.Cmd { return nil }

func (m browserModel[P, C]) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = max(40, msg.Width)
		m.height = max(8, msg.Height)
	case loaded[C]:
		m.loading = false
		m.err = msg.err
		m.children = msg.items
		m.rightIndex = 0
		m.rightFilter = ""
		if msg.err == nil && len(msg.items) > 0 {
			m.focusRight = true
		}
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "/":
			m.filtering = true
			return m, nil
		case "tab":
			if !m.focusRight && len(m.options.ParentGroups) > 1 {
				m.groupIndex = (m.groupIndex + 1) % len(m.options.ParentGroups)
				m.leftIndex = 0
				m.notice = ""
			}
		case "c":
			if m.focusRight {
				m.cachedOnly = !m.cachedOnly
				m.rightIndex = 0
				m.notice = ""
			}
		case "v":
			if m.focusRight {
				m.quality = nextQuality(m.quality)
				m.rightIndex = 0
				m.notice = ""
				if m.options.SaveQuality != nil {
					if err := m.options.SaveQuality(m.quality); err != nil {
						m.notice = "Could not save quality preference"
					}
				}
			}
		case "esc", "left", "h":
			if m.focusRight {
				m.focusRight = false
				return m, nil
			}
			return m, tea.Quit
		case "right", "l":
			if len(m.filteredChildren()) > 0 {
				m.focusRight = true
			}
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-m.pageSize())
		case "pgdown":
			m.move(m.pageSize())
		case "enter":
			if m.focusRight {
				children := m.filteredChildren()
				if len(children) > 0 {
					if stream, ok := any(children[m.rightIndex].item).(streamItem); ok && !stream.IsCached() {
						m.notice = "Uncached playback is not available yet"
						return m, nil
					}
					m.choice = children[m.rightIndex].item
					m.chosen = true
					return m, tea.Quit
				}
				return m, nil
			}
			parents := m.filteredParents()
			if len(parents) == 0 || m.loading {
				return m, nil
			}
			parent := parents[m.leftIndex].item
			m.openedLabel = plainLabel(parent.Label())
			m.loading = true
			m.err = nil
			m.children = nil
			return m, func() tea.Msg {
				items, err := m.load(m.ctx, parent)
				return loaded[C]{items: items, err: err}
			}
		}
	}
	return m, nil
}

func (m browserModel[P, C]) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filter := &m.leftFilter
	if m.focusRight {
		filter = &m.rightFilter
	}
	switch msg.String() {
	case "enter":
		m.filtering = false
	case "esc":
		*filter = ""
		m.filtering = false
	case "backspace", "ctrl+h":
		if len(*filter) > 0 {
			runes := []rune(*filter)
			*filter = string(runes[:len(runes)-1])
		}
	case "ctrl+w":
		*filter = strings.TrimRight(*filter, " ")
		if end := strings.LastIndex(*filter, " "); end >= 0 {
			*filter = strings.TrimRight((*filter)[:end+1], " ")
		} else {
			*filter = ""
		}
	case "ctrl+u":
		*filter = ""
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeyRunes {
			*filter += string(msg.Runes)
		}
	}
	if m.focusRight {
		m.rightIndex = clamp(m.rightIndex, len(m.filteredChildren()))
	} else {
		m.leftIndex = clamp(m.leftIndex, len(m.filteredParents()))
	}
	return m, nil
}

func (m *browserModel[P, C]) move(delta int) {
	if m.focusRight {
		m.rightIndex = clamp(m.rightIndex+delta, len(m.filteredChildren()))
		return
	}
	m.leftIndex = clamp(m.leftIndex+delta, len(m.filteredParents()))
}

func (m browserModel[P, C]) pageSize() int { return max(1, m.height-6) }

func (m browserModel[P, C]) filteredParents() []indexed[P] {
	items := filterItems(m.parents, m.leftFilter)
	if len(m.options.ParentGroups) == 0 {
		return items
	}
	group := m.options.ParentGroups[m.groupIndex]
	result := items[:0]
	for _, value := range items {
		if grouped, ok := any(value.item).(groupedItem); ok && grouped.Group() == group {
			result = append(result, value)
		}
	}
	return result
}

func (m browserModel[P, C]) filteredChildren() []indexed[C] {
	items := filterItems(m.children, m.rightFilter)
	result := items[:0]
	for _, value := range items {
		stream, ok := any(value.item).(streamItem)
		if ok && m.cachedOnly && !stream.IsCached() {
			continue
		}
		if ok && m.quality != 0 && stream.VideoQuality() != m.quality {
			continue
		}
		result = append(result, value)
	}
	return result
}

func filterItems[T item](items []T, query string) []indexed[T] {
	query = strings.ToLower(strings.TrimSpace(query))
	result := make([]indexed[T], 0, len(items))
	for i, value := range items {
		if query == "" || strings.Contains(strings.ToLower(plainLabel(value.Label())), query) {
			result = append(result, indexed[T]{index: i, item: value})
		}
	}
	return result
}

func (m browserModel[P, C]) View() string {
	width := m.width
	if width <= 0 {
		width = 100
	}
	height := m.height
	if height <= 0 {
		height = 24
	}
	leftWidth := max(18, width/2-2)
	rightWidth := max(18, width-leftWidth-3)
	rows := max(1, height-6)

	parents := m.filteredParents()
	leftTitle := fmt.Sprintf("Search results  %d", len(parents))
	if len(m.options.ParentGroups) > 0 {
		leftTitle = groupTabs(m.options.ParentGroups, m.groupIndex) + fmt.Sprintf("  %d", len(parents))
	}
	left := renderBrowserPane(leftTitle, parents, m.leftIndex, leftWidth, rows, !m.focusRight, m.leftFilter, false, nil)

	children := m.filteredChildren()
	cacheLabel := "Cached"
	if !m.cachedOnly {
		cacheLabel = "All"
	}
	qualityLabel := "All qualities"
	if m.quality > 0 {
		qualityLabel = fmt.Sprintf("%dp", m.quality)
	}
	rightTitle := fmt.Sprintf("Torrents  [%s]  [%s]", cacheLabel, qualityLabel)
	right := renderBrowserPane(rightTitle, children, m.rightIndex, rightWidth, rows, m.focusRight, m.rightFilter, m.loading, m.err)

	breadcrumb := "Search"
	if m.openedLabel != "" {
		breadcrumb += " / " + m.openedLabel
	}
	filterHint := ""
	if m.filtering {
		filter := m.leftFilter
		if m.focusRight {
			filter = m.rightFilter
		}
		filterHint = headerStyle.Render("Filter: "+filter+"_") + "\n"
	}
	helpText := "tab movie/series  h/l focus  j/k move  enter open  / filter  esc back"
	if m.focusRight {
		helpText = "c cached/all  v quality  h/l focus  j/k move  enter select  / filter  esc back"
	}
	if m.notice != "" {
		helpText = m.notice + "  |  " + helpText
	}
	help := hintStyle.Render(helpText)
	return ansi.Truncate(breadcrumb, width, "...") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + filterHint + help + "\n"
}

func renderBrowserPane[T item](title string, items []indexed[T], selected, width, rows int, active bool, filter string, loading bool, loadErr error) string {
	contentWidth := max(1, width-2)
	lines := []string{headerStyle.Render(ansi.Truncate(title, contentWidth, "..."))}
	if loading {
		lines = append(lines, "Loading...")
	} else if loadErr != nil {
		lines = append(lines, ansi.Truncate("Error: "+loadErr.Error(), contentWidth, "..."), "Press Enter to retry")
	} else if len(items) == 0 {
		message := "No items"
		if filter != "" {
			message = "No filter matches"
		}
		lines = append(lines, message)
	} else {
		selected = clamp(selected, len(items))
		start := max(0, min(selected-rows/2, len(items)-rows))
		end := min(len(items), start+rows)
		for i := start; i < end; i++ {
			label := ansi.Truncate(plainLabel(items[i].item.Label()), max(1, contentWidth-2), "...")
			row := "  " + label
			if i == selected {
				row = selectedStyle.Width(contentWidth).Render("> " + label)
			}
			lines = append(lines, row)
		}
		lines[0] += hintStyle.Render(fmt.Sprintf("  %d-%d/%d", start+1, end, len(items)))
	}
	for len(lines) < rows+1 {
		lines = append(lines, "")
	}
	style := inactiveBorder
	if active {
		style = activeBorder
	}
	return style.Width(width).Height(rows + 1).Render(strings.Join(lines, "\n"))
}

func plainLabel(value string) string {
	value = strings.NewReplacer("\r", " ", "\n", " ", "\t", " ").Replace(value)
	return strings.Join(strings.Fields(ansi.Strip(value)), " ")
}

func clamp(value, length int) int {
	if length <= 0 {
		return 0
	}
	return min(max(0, value), length-1)
}

func groupTabs(groups []string, active int) string {
	labels := make([]string, len(groups))
	for i, group := range groups {
		label := strings.ToUpper(group[:1]) + group[1:]
		if i == active {
			label = "[" + label + "]"
		}
		labels[i] = label
	}
	return strings.Join(labels, " ")
}

func nextQuality(current int) int {
	qualities := []int{0, 2160, 1080, 720, 480}
	for i, quality := range qualities {
		if quality == current {
			return qualities[(i+1)%len(qualities)]
		}
	}
	return 0
}

func Browse[P item, C item](ctx context.Context, input io.Reader, output io.Writer, parents []P, load Loader[P, C], options BrowserOptions) (C, error) {
	var zero C
	if len(parents) == 0 {
		return zero, errors.New("no choices")
	}
	initial := browserModel[P, C]{ctx: ctx, parents: parents, load: load, options: options, cachedOnly: true, quality: options.PreferredQuality, width: 100, height: 24}
	program := tea.NewProgram(initial, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output))
	final, err := program.Run()
	if err != nil {
		return zero, err
	}
	model := final.(browserModel[P, C])
	if !model.chosen {
		return zero, ErrCancelled
	}
	return model.choice, nil
}
