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

type BrowserOptions[T item] struct {
	InitialTitle     string
	ParentGroups     []string
	PreferredGroup   string
	PreferredQuality int
	SaveGroup        func(string) error
	SaveQuality      func(int) error
	ChildTitle       func(T) string
	Play             func(context.Context, T) error
}

type groupedItem interface{ Group() string }
type terminalItem interface{ Terminal() bool }
type streamItem interface {
	StreamInfo() (cached bool, quality int, ok bool)
}

type indexed[T item] struct {
	index int
	item  T
}

type pane[T item] struct {
	title  string
	items  []T
	index  int
	filter string
}

type loaded[T item] struct {
	items []T
	err   error
}

type playFinished struct{ err error }

type browserModel[T item] struct {
	ctx         context.Context
	levels      []pane[T]
	right       pane[T]
	load        func(context.Context, T) ([]T, error)
	options     BrowserOptions[T]
	crumbs      []string
	groupIndex  int
	focusRight  bool
	loading     bool
	err         error
	filtering   bool
	cachedOnly  bool
	quality     int
	notice      string
	width       int
	height      int
	chosen      bool
	choice      T
	playing     bool
	stopPlaying context.CancelFunc
}

var (
	activeBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"})
	inactiveBorder = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: "#555555"})
)

func (m browserModel[T]) Init() tea.Cmd { return nil }

func (m browserModel[T]) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = max(40, msg.Width)
		m.height = max(8, msg.Height)
	case loaded[T]:
		m.loading = false
		m.err = msg.err
		m.right.items = msg.items
		m.right.index = 0
		m.right.filter = ""
		if msg.err == nil && len(msg.items) > 0 {
			m.focusRight = true
		}
	case playFinished:
		m.playing = false
		m.stopPlaying = nil
		if msg.err == nil {
			m.notice = "Playback launched"
		} else if errors.Is(msg.err, context.Canceled) {
			m.notice = "Playback stopped"
		} else {
			m.notice = "Playback failed: " + msg.err.Error()
		}
	case tea.KeyMsg:
		if m.filtering {
			return m.updateFilter(msg)
		}
		switch msg.String() {
		case "ctrl+c", "q":
			if m.stopPlaying != nil {
				m.stopPlaying()
			}
			return m, tea.Quit
		case "s":
			if m.stopPlaying != nil {
				m.stopPlaying()
				m.notice = "Stopping playback..."
			}
		case "/":
			m.filtering = true
		case "tab":
			if !m.focusRight && len(m.levels) == 1 && len(m.options.ParentGroups) > 1 {
				m.groupIndex = (m.groupIndex + 1) % len(m.options.ParentGroups)
				m.current().index = 0
				m.notice = ""
				if m.options.SaveGroup != nil {
					if err := m.options.SaveGroup(m.options.ParentGroups[m.groupIndex]); err != nil {
						m.notice = "Could not save media tab preference"
					}
				}
			}
		case "c":
			if m.focusRight && m.rightHasStreams() {
				m.cachedOnly = !m.cachedOnly
				m.right.index = 0
				m.notice = ""
			}
		case "v":
			if m.focusRight && m.rightHasStreams() {
				m.quality = nextQuality(m.quality)
				m.right.index = 0
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
			} else if len(m.levels) > 1 {
				popped := m.levels[len(m.levels)-1]
				m.levels = m.levels[:len(m.levels)-1]
				m.right = popped
				m.crumbs = m.crumbs[:max(0, len(m.crumbs)-1)]
				m.focusRight = true
				m.err = nil
				m.notice = ""
			} else {
				return m, tea.Quit
			}
		case "right", "l":
			if len(m.filteredRight()) > 0 {
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
			return m.confirm()
		}
	}
	return m, nil
}

func (m browserModel[T]) confirm() (tea.Model, tea.Cmd) {
	if m.loading {
		return m, nil
	}
	if m.focusRight {
		items := m.filteredRight()
		if len(items) == 0 {
			return m, nil
		}
		selected := items[m.right.index].item
		if terminal, ok := any(selected).(terminalItem); ok && terminal.Terminal() {
			if stream, ok := any(selected).(streamItem); ok {
				cached, _, isStream := stream.StreamInfo()
				if isStream && !cached {
					m.notice = "Uncached playback is not available yet"
					return m, nil
				}
			}
			if m.options.Play == nil {
				m.choice = selected
				m.chosen = true
				return m, tea.Quit
			}
			if m.playing {
				m.notice = "Stop current playback before starting another"
				return m, nil
			}
			playContext, cancel := context.WithCancel(m.ctx)
			m.playing = true
			m.stopPlaying = cancel
			m.notice = "Starting playback..."
			return m, func() tea.Msg {
				return playFinished{err: m.options.Play(playContext, selected)}
			}
		}
		m.levels = append(m.levels, m.right)
		m.focusRight = false
	}
	items := m.filteredCurrent()
	if len(items) == 0 {
		return m, nil
	}
	selected := items[m.current().index].item
	m.crumbs = append(m.crumbs[:len(m.levels)-1], plainLabel(selected.Label()))
	m.right = pane[T]{title: m.childTitle(selected)}
	m.loading = true
	m.err = nil
	m.notice = ""
	return m, func() tea.Msg {
		items, err := m.load(m.ctx, selected)
		return loaded[T]{items: items, err: err}
	}
}

func (m browserModel[T]) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filter := &m.current().filter
	if m.focusRight {
		filter = &m.right.filter
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
		m.right.index = clamp(m.right.index, len(m.filteredRight()))
	} else {
		m.current().index = clamp(m.current().index, len(m.filteredCurrent()))
	}
	return m, nil
}

func (m *browserModel[T]) move(delta int) {
	if m.focusRight {
		m.right.index = clamp(m.right.index+delta, len(m.filteredRight()))
	} else {
		m.current().index = clamp(m.current().index+delta, len(m.filteredCurrent()))
	}
}

func (m *browserModel[T]) current() *pane[T] { return &m.levels[len(m.levels)-1] }
func (m browserModel[T]) pageSize() int      { return max(1, m.height-6) }

func (m browserModel[T]) filteredCurrent() []indexed[T] {
	current := m.levels[len(m.levels)-1]
	items := filterItems(current.items, current.filter)
	if len(m.levels) != 1 || len(m.options.ParentGroups) == 0 {
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

func (m browserModel[T]) filteredRight() []indexed[T] {
	items := filterItems(m.right.items, m.right.filter)
	result := items[:0]
	for _, value := range items {
		stream, ok := any(value.item).(streamItem)
		cached, quality, isStream := false, 0, false
		if ok {
			cached, quality, isStream = stream.StreamInfo()
		}
		if isStream && m.cachedOnly && !cached {
			continue
		}
		if isStream && m.quality != 0 && quality != m.quality {
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

func (m browserModel[T]) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 24
	}
	leftWidth := max(18, width/2-2)
	rightWidth := max(18, width-leftWidth-3)
	rows := max(1, height-6)
	current := m.levels[len(m.levels)-1]
	leftTitle := current.title
	if len(m.levels) == 1 && len(m.options.ParentGroups) > 0 {
		leftTitle = groupTabs(m.options.ParentGroups, m.groupIndex)
	}
	leftItems := m.filteredCurrent()
	left := renderBrowserPane(leftTitle, leftItems, current.index, leftWidth, rows, !m.focusRight, current.filter, false, nil)
	rightItems := m.filteredRight()
	rightTitle := m.right.title
	if m.rightHasStreams() {
		cacheLabel := "Cached"
		if !m.cachedOnly {
			cacheLabel = "All"
		}
		qualityLabel := "All qualities"
		if m.quality > 0 {
			qualityLabel = fmt.Sprintf("%dp", m.quality)
		}
		rightTitle = fmt.Sprintf("Torrents  [%s]  [%s]", cacheLabel, qualityLabel)
	}
	right := renderBrowserPane(rightTitle, rightItems, m.right.index, rightWidth, rows, m.focusRight, m.right.filter, m.loading, m.err)
	breadcrumb := "Search"
	if len(m.crumbs) > 0 {
		breadcrumb += " / " + strings.Join(m.crumbs, " / ")
	}
	filterHint := ""
	if m.filtering {
		filter := current.filter
		if m.focusRight {
			filter = m.right.filter
		}
		filterHint = headerStyle.Render("Filter: "+filter+"_") + "\n"
	}
	helpText := "tab movie/series  h/l focus  j/k move  enter open  / filter  esc back"
	if m.focusRight {
		helpText = "h/l focus  j/k move  enter open/select  / filter  esc back"
		if m.rightHasStreams() {
			helpText = "c cached/all  v quality  " + helpText
		}
	}
	if m.notice != "" {
		helpText = m.notice + "  |  " + helpText
	}
	if m.playing {
		helpText = "PLAYING  s stop  |  " + helpText
	}
	return ansi.Truncate(breadcrumb, width, "...") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, left, right) + "\n" + filterHint + hintStyle.Render(helpText) + "\n"
}

func (m browserModel[T]) rightHasStreams() bool {
	for _, value := range m.right.items {
		if stream, ok := any(value).(streamItem); ok {
			_, _, isStream := stream.StreamInfo()
			if isStream {
				return true
			}
		}
	}
	return false
}

func (m browserModel[T]) childTitle(value T) string {
	if m.options.ChildTitle != nil {
		return m.options.ChildTitle(value)
	}
	return "Items"
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

func Browse[T item](ctx context.Context, input io.Reader, output io.Writer, items []T, load func(context.Context, T) ([]T, error), options BrowserOptions[T]) (T, error) {
	var zero T
	if len(items) == 0 {
		return zero, errors.New("no choices")
	}
	title := options.InitialTitle
	if title == "" {
		title = "Search results"
	}
	groupIndex := preferredGroupIndex(options.ParentGroups, options.PreferredGroup)
	initial := browserModel[T]{ctx: ctx, levels: []pane[T]{{title: title, items: items}}, load: load, options: options, groupIndex: groupIndex, cachedOnly: true, quality: options.PreferredQuality, width: 100, height: 24}
	program := tea.NewProgram(initial, tea.WithContext(ctx), tea.WithInput(input), tea.WithOutput(output))
	final, err := program.Run()
	if err != nil {
		return zero, err
	}
	model := final.(browserModel[T])
	if options.Play != nil {
		return zero, nil
	}
	if !model.chosen {
		return zero, ErrCancelled
	}
	return model.choice, nil
}

func preferredGroupIndex(groups []string, preferred string) int {
	for i, group := range groups {
		if group == preferred {
			return i
		}
	}
	return 0
}
