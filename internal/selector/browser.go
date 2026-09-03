package selector

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type BrowserOptions[T item] struct {
	InitialTitle      string
	InitialQuery      string
	ParentGroups      []string
	PreferredGroup    string
	PreferredQuality  int
	PreferredCached   *bool
	PreferredProvider string
	PreferredPlayer   string
	Providers         []string
	PreferredModes    map[string]string
	ModeOptions       map[string][]ContextMode
	SaveGroup         func(string) error
	SaveQuality       func(int) error
	SaveCached        func(bool) error
	SaveProvider      func(string) error
	SavePlayer        func(string) error
	SaveMode          func(string, string) error
	ChildTitle        func(T) string
	Play              func(context.Context, T) error
	Requery           func(context.Context, string) ([]T, error)
	History           func(context.Context) ([]T, error)
	ToggleHistory     func(context.Context, T) (bool, error)
	RemoveHistory     func(context.Context, T) error
	SearchGroups      []string
}

type groupedItem interface{ Group() string }
type terminalItem interface{ Terminal() bool }
type streamItem interface {
	StreamInfo() (StreamInfo, bool)
}

type StreamInfo struct {
	Quality         int
	Cached          bool
	CacheApplicable bool
	Playable        bool
}
type sortableItem interface {
	SortFields() (name string, year int, ok bool)
}

type sortMode int

const (
	sortRelevance sortMode = iota
	sortNameAscending
	sortNameDescending
	sortYearAscending
	sortYearDescending
	sortQualityAscending
	sortQualityDescending
	sortCachedFirst
	sortUncachedFirst
)

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

type visiblePane[T item] struct {
	title   string
	items   []indexed[T]
	index   int
	filter  string
	active  bool
	loading bool
	err     error
}

type loaded[T item] struct {
	items    []T
	err      error
	key      string
	provider string
	loadID   uint64
}

type playFinished struct{ err error }
type requeryFinished[T item] struct {
	items []T
	err   error
	query string
}
type historyFinished[T item] struct {
	items []T
	err   error
}
type historyChanged[T item] struct {
	items []T
	added bool
	err   error
}

type helpBinding struct {
	keys  string
	label string
	key   tea.KeyMsg
}

type ContextMode struct {
	Group, Key, Name, Value string
}

type contextualItem interface {
	ContextModes() []ContextMode
}
type unavailableItem interface{ Unavailable() bool }
type cacheableItem interface{ CacheKey() string }

type toastExpired struct{ id uint64 }
type spinnerTick struct{}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type browserModel[T item] struct {
	ctx               context.Context
	levels            []pane[T]
	right             pane[T]
	load              func(context.Context, T) ([]T, error)
	options           BrowserOptions[T]
	crumbs            []string
	groupIndex        int
	focusRight        bool
	loading           bool
	searching         bool
	historyBusy       bool
	spinnerFrame      int
	err               error
	filtering         bool
	querying          bool
	query             string
	activeQuery       string
	sortMenu          bool
	modeMenu          bool
	mode              map[string]string
	sortMode          sortMode
	streamSort        sortMode
	helpMenu          bool
	helpFilter        string
	helpIndex         int
	settingsMenu      bool
	settingsIndex     int
	player            string
	provider          string
	customPlayer      bool
	customPlayerValue string
	pendingG          bool
	cachedOnly        bool
	quality           int
	notice            string
	toastID           uint64
	width             int
	height            int
	chosen            bool
	choice            T
	playing           bool
	stopPlaying       context.CancelFunc
	loadCache         map[string][]T
	loadID            uint64
}

var (
	activeBorder     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"})
	inactiveBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: "#555555"})
	toastBorder      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#FF6B6B"}).Padding(0, 1)
	unavailableStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#666666"})
)

func (m browserModel[T]) Init() tea.Cmd { return nil }

func (m browserModel[T]) Update(message tea.Msg) (result tea.Model, command tea.Cmd) {
	previousNotice := m.notice
	defer func() {
		updated, ok := result.(browserModel[T])
		if !ok {
			return
		}
		if updated.notice != "" && updated.notice != previousNotice {
			updated.toastID++
			id := updated.toastID
			if command == nil {
				command = tea.Tick(4*time.Second, func(time.Time) tea.Msg { return toastExpired{id: id} })
			}
		}
		result = updated
	}()
	switch msg := message.(type) {
	case tea.WindowSizeMsg:
		m.width = max(40, msg.Width)
		m.height = max(8, msg.Height)
	case loaded[T]:
		if msg.provider != m.provider || msg.loadID != m.loadID {
			break
		}
		m.loading = false
		m.err = msg.err
		m.right.items = msg.items
		m.right.index = 0
		m.right.filter = ""
		if msg.err == nil && msg.key != "" {
			if m.loadCache == nil {
				m.loadCache = make(map[string][]T)
			}
			m.loadCache[msg.key] = msg.items
		}
		if msg.err == nil && len(msg.items) > 0 {
			m.focusRight = true
		} else if msg.err != nil {
			m.notice = "Load failed: " + msg.err.Error()
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
	case requeryFinished[T]:
		m.loading = false
		m.searching = false
		if msg.err != nil {
			m.notice = "Search failed: " + msg.err.Error()
			break
		}
		title := "Search results"
		m.levels = []pane[T]{{title: title, items: msg.items}}
		m.options.ParentGroups = m.options.SearchGroups
		m.right = pane[T]{}
		m.crumbs = nil
		m.activeQuery = msg.query
		m.focusRight = false
		m.notice = ""
	case historyFinished[T]:
		m.loading = false
		if msg.err != nil {
			m.notice = "History failed: " + msg.err.Error()
			break
		}
		m.levels = []pane[T]{{title: "History", items: msg.items}}
		m.right = pane[T]{}
		m.crumbs = nil
		m.options.ParentGroups = nil
		m.activeQuery = "History"
		m.focusRight = false
		m.notice = ""
	case historyChanged[T]:
		m.historyBusy = false
		if msg.err != nil {
			m.notice = "History update failed: " + msg.err.Error()
			break
		}
		if m.inHistoryRoot() {
			m.current().items = msg.items
			m.current().index = clamp(m.current().index, len(msg.items))
		}
		if msg.added {
			m.notice = "Added to history"
		} else {
			m.notice = "Removed from history"
		}
	case spinnerTick:
		if m.searching || m.historyBusy {
			m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
			return m, spinnerCommand()
		}
	case toastExpired:
		if msg.id == m.toastID {
			m.notice = ""
		}
	case tea.KeyMsg:
		if m.searching || m.historyBusy {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.helpMenu {
			return m.updateHelp(msg)
		}
		if m.customPlayer {
			return m.updateCustomPlayer(msg)
		}
		if m.settingsMenu {
			return m.updateSettings(msg)
		}
		if m.sortMenu {
			return m.updateSort(msg)
		}
		if m.modeMenu {
			return m.updateMode(msg)
		}
		if m.querying {
			return m.updateQuery(msg)
		}
		if m.filtering {
			return m.updateFilter(msg)
		}
		if m.pendingG {
			m.pendingG = false
			if msg.String() == "g" {
				m.move(-1 << 30)
				return m, nil
			}
		}
		switch msg.String() {
		case "ctrl+c", "q":
			if m.stopPlaying != nil {
				m.stopPlaying()
			}
			return m, tea.Quit
		case "x":
			if m.stopPlaying != nil {
				m.stopPlaying()
				m.notice = "Stopping playback..."
			}
		case "g":
			m.pendingG = true
		case "G":
			m.move(1 << 30)
		case "r":
			if !m.loading {
				items := m.filteredCurrent()
				if len(items) > 0 {
					selected := items[m.current().index].item
					if cacheable, ok := any(selected).(cacheableItem); ok && cacheable.CacheKey() != "" {
						return m.loadSelected(selected, true)
					}
				}
			}
		case "s":
			if (!m.focusRight && len(m.levels) == 1) || (m.focusRight && m.rightHasStreams()) {
				m.sortMenu = true
			}
		case "m":
			if len(m.contextModes()) > 0 {
				m.modeMenu = true
			}
		case "/":
			m.filtering = true
		case "?":
			m.helpMenu = true
			m.helpFilter = ""
			m.helpIndex = 0
		case ";":
			m.settingsMenu = true
			m.settingsIndex = 0
		case "ctrl+p":
			if m.options.Requery != nil && !m.loading {
				m.querying = true
				m.query = ""
			}
		case "ctrl+h":
			if m.options.History != nil && !m.loading {
				m.loading = true
				return m, func() tea.Msg {
					items, err := m.options.History(m.ctx)
					return historyFinished[T]{items: items, err: err}
				}
			}
		case "w":
			if selected, ok := m.selectedRoot(); ok && m.options.ToggleHistory != nil && m.options.History != nil {
				m.historyBusy = true
				return m, tea.Batch(func() tea.Msg {
					added, err := m.options.ToggleHistory(m.ctx, selected)
					if err != nil {
						return historyChanged[T]{err: err}
					}
					items, err := m.options.History(m.ctx)
					return historyChanged[T]{items: items, added: added, err: err}
				}, spinnerCommand())
			}
		case "d":
			if selected, ok := m.selectedRoot(); ok && m.inHistoryRoot() && m.options.RemoveHistory != nil && m.options.History != nil {
				m.historyBusy = true
				return m, tea.Batch(func() tea.Msg {
					if err := m.options.RemoveHistory(m.ctx, selected); err != nil {
						return historyChanged[T]{err: err}
					}
					items, err := m.options.History(m.ctx)
					return historyChanged[T]{items: items, err: err}
				}, spinnerCommand())
			}
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
			if m.focusRight && m.rightHasStreams() && m.rightCacheApplicable() {
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
		case "esc":
			if m.focusRight && m.right.filter != "" {
				m.right.filter = ""
				m.right.index = 0
			} else if !m.focusRight && m.current().filter != "" {
				m.current().filter = ""
				m.current().index = 0
			} else {
				m.back()
			}
		case "left", "h":
			m.back()
		case "right", "l":
			if !m.loading {
				return m.confirm()
			}
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.move(-m.pageSize())
		case "pgdown":
			m.move(m.pageSize())
		case "ctrl+d":
			m.move(max(1, m.pageSize()/2))
		case "ctrl+u":
			m.move(-max(1, m.pageSize()/2))
		case "enter":
			return m.confirm()
		}
	}
	return m, nil
}

func (m browserModel[T]) contextModes() []ContextMode {
	var items []indexed[T]
	if m.focusRight {
		items = m.filteredRight()
	} else {
		items = m.filteredCurrent()
	}
	if len(items) == 0 {
		return nil
	}
	contextual, ok := any(items[0].item).(contextualItem)
	if !ok {
		return nil
	}
	return contextual.ContextModes()
}

func (m browserModel[T]) updateMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.modeMenu = false
	if msg.String() == "esc" {
		return m, nil
	}
	modes := m.contextModes()
	for _, mode := range modes {
		if msg.String() == mode.Key {
			m.setContextMode(mode.Key)
			return m, nil
		}
	}
	return m, nil
}

func modeModal(modes []ContextMode) string {
	lines := []string{headerStyle.Render("Mode"), ""}
	for _, mode := range modes {
		lines = append(lines, fmt.Sprintf("[%s] %s", mode.Key, mode.Name))
	}
	lines = append(lines, "", hintStyle.Render("Choose mode  Esc cancel"))
	return activeBorder.Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m browserModel[T]) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	bindings := m.filteredHelpBindings()
	switch msg.String() {
	case "esc":
		m.helpMenu = false
		m.helpFilter = ""
	case "up":
		m.helpIndex = clamp(m.helpIndex-1, len(bindings))
	case "down":
		m.helpIndex = clamp(m.helpIndex+1, len(bindings))
	case "enter":
		if len(bindings) == 0 {
			return m, nil
		}
		selected := bindings[clamp(m.helpIndex, len(bindings))]
		m.helpMenu = false
		m.helpFilter = ""
		m.helpIndex = 0
		if selected.keys == "gg" {
			m.move(-1 << 30)
			return m, nil
		}
		return m.Update(selected.key)
	case "backspace", "ctrl+h":
		if len(m.helpFilter) > 0 {
			runes := []rune(m.helpFilter)
			m.helpFilter = string(runes[:len(runes)-1])
		}
	case "ctrl+w":
		m.helpFilter = strings.TrimRight(m.helpFilter, " ")
		if end := strings.LastIndex(m.helpFilter, " "); end >= 0 {
			m.helpFilter = strings.TrimRight(m.helpFilter[:end+1], " ")
		} else {
			m.helpFilter = ""
		}
	case "ctrl+u":
		m.helpFilter = ""
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeySpace {
			m.helpFilter += " "
		} else if msg.Type == tea.KeyRunes {
			m.helpFilter += string(msg.Runes)
		}
	}
	m.helpIndex = clamp(m.helpIndex, len(m.filteredHelpBindings()))
	return m, nil
}

var settingModeGroups = []string{"media", "season", "episode", "stream"}

func (m browserModel[T]) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", ";":
		m.settingsMenu = false
	case "up", "k":
		m.settingsIndex = clamp(m.settingsIndex-1, 9)
	case "down", "j":
		m.settingsIndex = clamp(m.settingsIndex+1, 9)
	case "left", "h":
		m.changeSetting(-1)
	case "right", "l":
		m.changeSetting(1)
	case "enter":
		if m.settingsIndex == 4 {
			m.customPlayer = true
			m.customPlayerValue = m.player
			if m.player == "mpv" || m.player == "vlc" {
				m.customPlayerValue = ""
			}
		} else {
			m.changeSetting(1)
		}
	case "ctrl+c":
		return m, tea.Quit
	}
	return m, nil
}

func (m *browserModel[T]) changeSetting(delta int) {
	switch m.settingsIndex {
	case 0:
		if len(m.options.ParentGroups) > 1 {
			m.groupIndex = wrapIndex(m.groupIndex+delta, len(m.options.ParentGroups))
			if m.options.SaveGroup != nil {
				m.saveSetting(m.options.SaveGroup(m.options.ParentGroups[m.groupIndex]))
			}
		}
	case 1:
		qualities := []int{0, 2160, 1080, 720, 480}
		index := 0
		for i, quality := range qualities {
			if quality == m.quality {
				index = i
			}
		}
		m.quality = qualities[wrapIndex(index+delta, len(qualities))]
		if m.options.SaveQuality != nil {
			m.saveSetting(m.options.SaveQuality(m.quality))
		}
	case 2:
		m.cachedOnly = !m.cachedOnly
		if m.options.SaveCached != nil {
			m.saveSetting(m.options.SaveCached(m.cachedOnly))
		}
	case 3:
		if len(m.options.Providers) == 0 {
			return
		}
		index := 0
		for i, provider := range m.options.Providers {
			if provider == m.provider {
				index = i
			}
		}
		m.provider = m.options.Providers[wrapIndex(index+delta, len(m.options.Providers))]
		m.right = pane[T]{}
		m.focusRight = false
		m.loading = false
		m.loadCache = nil
		m.loadID++
		if m.options.SaveProvider != nil {
			m.saveSetting(m.options.SaveProvider(m.provider))
		}
	case 4:
		players := []string{"", "mpv", "vlc"}
		index := 0
		for i, player := range players {
			if player == m.player {
				index = i
			}
		}
		if m.player != "" && m.player != "mpv" && m.player != "vlc" {
			players = append(players, m.player)
			index = len(players) - 1
		}
		m.player = players[wrapIndex(index+delta, len(players))]
		if m.options.SavePlayer != nil {
			m.saveSetting(m.options.SavePlayer(m.player))
		}
	default:
		group := settingModeGroups[m.settingsIndex-5]
		modes := m.options.ModeOptions[group]
		if len(modes) == 0 {
			return
		}
		index := 0
		for i, mode := range modes {
			if mode.Key == m.mode[group] {
				index = i
			}
		}
		if m.mode == nil {
			m.mode = make(map[string]string)
		}
		m.mode[group] = modes[wrapIndex(index+delta, len(modes))].Key
		if m.options.SaveMode != nil {
			m.saveSetting(m.options.SaveMode(group, m.mode[group]))
		}
	}
}

func (m *browserModel[T]) saveSetting(err error) {
	if err != nil {
		m.notice = "Could not save preference"
	}
}

func (m browserModel[T]) updateCustomPlayer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.customPlayer = false
	case "enter":
		m.player = strings.TrimSpace(m.customPlayerValue)
		m.customPlayer = false
		if m.options.SavePlayer != nil {
			m.saveSetting(m.options.SavePlayer(m.player))
		}
	case "backspace", "ctrl+h":
		if len(m.customPlayerValue) > 0 {
			runes := []rune(m.customPlayerValue)
			m.customPlayerValue = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.customPlayerValue = ""
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeySpace {
			m.customPlayerValue += " "
		} else if msg.Type == tea.KeyRunes {
			m.customPlayerValue += string(msg.Runes)
		}
	}
	return m, nil
}

func wrapIndex(index, length int) int {
	if length == 0 {
		return 0
	}
	return (index%length + length) % length
}

func (m browserModel[T]) filteredHelpBindings() []helpBinding {
	bindings := []helpBinding{
		{keys: "Enter / Right / l", label: "Open or confirm", key: tea.KeyMsg{Type: tea.KeyEnter}},
		{keys: "Left / h / Esc", label: "Go back", key: tea.KeyMsg{Type: tea.KeyEscape}},
		{keys: "Up / k", label: "Move up", key: tea.KeyMsg{Type: tea.KeyUp}},
		{keys: "Down / j", label: "Move down", key: tea.KeyMsg{Type: tea.KeyDown}},
		{keys: "PgUp", label: "Previous page", key: tea.KeyMsg{Type: tea.KeyPgUp}},
		{keys: "PgDown", label: "Next page", key: tea.KeyMsg{Type: tea.KeyPgDown}},
		{keys: "Ctrl-D", label: "Move half-page down", key: tea.KeyMsg{Type: tea.KeyCtrlD}},
		{keys: "Ctrl-U", label: "Move half-page up", key: tea.KeyMsg{Type: tea.KeyCtrlU}},
		{keys: "gg", label: "Move to first item", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g', 'g'}}},
		{keys: "G", label: "Move to last item", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}},
		{keys: "r", label: "Refresh episode torrents", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}},
		{keys: "/", label: "Filter active pane", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}},
		{keys: "s", label: "Sort active results", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}},
		{keys: "m", label: "Choose detail mode", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}},
		{keys: "x", label: "Stop playback", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}},
		{keys: "c", label: "Toggle cached or all", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}},
		{keys: "v", label: "Cycle video quality", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}},
		{keys: "?", label: "Show keybindings", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}},
		{keys: ";", label: "Settings", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{';'}}},
		{keys: "q", label: "Quit", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
	}
	if len(m.options.ParentGroups) > 1 {
		bindings = append(bindings, helpBinding{keys: "Tab", label: "Toggle movie or series", key: tea.KeyMsg{Type: tea.KeyTab}})
	}
	if m.options.Requery != nil {
		bindings = append(bindings, helpBinding{keys: "Ctrl-P", label: "Run new search", key: tea.KeyMsg{Type: tea.KeyCtrlP}})
	}
	if m.options.History != nil {
		bindings = append(bindings, helpBinding{keys: "Ctrl-H", label: "Open history", key: tea.KeyMsg{Type: tea.KeyCtrlH}})
	}
	if m.options.ToggleHistory != nil && m.options.History != nil {
		bindings = append(bindings, helpBinding{keys: "w", label: "Toggle selected title in history", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}}})
	}
	if m.inHistoryRoot() && m.options.RemoveHistory != nil && m.options.History != nil {
		bindings = append(bindings, helpBinding{keys: "d", label: "Remove selected title from history", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}}})
	}
	query := strings.ToLower(strings.TrimSpace(m.helpFilter))
	if query == "" {
		return bindings
	}
	result := make([]helpBinding, 0, len(bindings))
	for _, binding := range bindings {
		if strings.Contains(strings.ToLower(binding.keys+" "+binding.label), query) {
			result = append(result, binding)
		}
	}
	return result
}

func (m browserModel[T]) updateSort(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.sortMenu = false
	if m.focusRight && m.rightHasStreams() {
		switch msg.String() {
		case "d", "r":
			m.streamSort = sortRelevance
		case "q":
			m.streamSort = sortQualityAscending
			m.setContextMode("q")
		case "Q":
			m.streamSort = sortQualityDescending
			m.setContextMode("q")
		case "c":
			m.streamSort = sortCachedFirst
			m.setContextMode("c")
		case "C":
			m.streamSort = sortUncachedFirst
			m.setContextMode("c")
		case "n":
			m.streamSort = sortNameAscending
		case "N":
			m.streamSort = sortNameDescending
		case "ctrl+c":
			return m, tea.Quit
		case "esc":
			return m, nil
		default:
			m.notice = "Unknown torrent sort key"
		}
		m.right.index = 0
		return m, nil
	}
	switch msg.String() {
	case "a":
		m.sortMode = sortNameAscending
	case "A":
		m.sortMode = sortNameDescending
	case "y":
		m.sortMode = sortYearAscending
		m.setContextMode("y")
	case "Y":
		m.sortMode = sortYearDescending
		m.setContextMode("y")
	case "d", "r":
		m.sortMode = sortRelevance
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, nil
	default:
		m.notice = "Unknown sort key"
	}
	m.current().index = 0
	return m, nil
}

func (m *browserModel[T]) setContextMode(key string) {
	modes := m.contextModes()
	if len(modes) == 0 {
		return
	}
	if m.mode == nil {
		m.mode = make(map[string]string)
	}
	group := modes[0].Group
	if group == "" {
		group = modes[0].Name
	}
	m.mode[group] = key
	if m.options.SaveMode != nil {
		if err := m.options.SaveMode(group, key); err != nil {
			m.notice = "Could not save detail mode preference"
		}
	}
}

func (m *browserModel[T]) back() {
	if m.focusRight {
		m.focusRight = false
		return
	}
	if len(m.levels) <= 1 {
		return
	}
	popped := m.levels[len(m.levels)-1]
	m.levels = m.levels[:len(m.levels)-1]
	m.right = popped
	m.crumbs = m.crumbs[:max(0, len(m.crumbs)-1)]
	m.focusRight = true
	m.err = nil
	m.notice = ""
}

func (m browserModel[T]) updateQuery(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.querying = false
		m.query = ""
	case "enter":
		query := strings.TrimSpace(m.query)
		if query == "" {
			return m, nil
		}
		m.querying = false
		m.loading = true
		m.searching = true
		m.spinnerFrame = 0
		m.notice = ""
		return m, tea.Batch(func() tea.Msg {
			items, err := m.options.Requery(m.ctx, query)
			return requeryFinished[T]{items: items, err: err, query: query}
		}, spinnerCommand())
	case "backspace", "ctrl+h":
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
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeySpace {
			m.query += " "
		} else if msg.Type == tea.KeyRunes {
			m.query += string(msg.Runes)
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
				info, isStream := stream.StreamInfo()
				if isStream && !info.Playable {
					m.notice = "Stream is not playable"
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
	return m.loadSelected(selected, false)
}

func (m browserModel[T]) loadSelected(selected T, refresh bool) (tea.Model, tea.Cmd) {
	key := ""
	if cacheable, ok := any(selected).(cacheableItem); ok {
		key = cacheable.CacheKey()
		if key != "" && m.provider != "" {
			key = m.provider + ":" + key
		}
	}
	m.right = pane[T]{title: m.childTitle(selected)}
	m.err = nil
	m.notice = ""
	if !refresh && key != "" {
		if cached, ok := m.loadCache[key]; ok {
			m.right.items = cached
			m.focusRight = len(cached) > 0
			m.loading = false
			return m, nil
		}
	}
	m.loading = true
	m.loadID++
	loadID := m.loadID
	return m, func() tea.Msg {
		items, err := m.load(m.ctx, selected)
		return loaded[T]{items: items, err: err, key: key, provider: m.provider, loadID: loadID}
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
		if msg.Type == tea.KeySpace {
			*filter += " "
		} else if msg.Type == tea.KeyRunes {
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

func (m browserModel[T]) inHistoryRoot() bool {
	return len(m.levels) == 1 && !m.focusRight && m.activeQuery == "History"
}

func (m browserModel[T]) selectedRoot() (T, bool) {
	var zero T
	if len(m.levels) != 1 || m.focusRight {
		return zero, false
	}
	items := m.filteredCurrent()
	if len(items) == 0 {
		return zero, false
	}
	return items[clamp(m.current().index, len(items))].item, true
}

func spinnerCommand() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerTick{} })
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
	if m.sortMode != sortRelevance {
		sort.SliceStable(result, func(i, j int) bool {
			left, leftOK := any(result[i].item).(sortableItem)
			right, rightOK := any(result[j].item).(sortableItem)
			if !leftOK || !rightOK {
				return false
			}
			leftName, leftYear, leftSortable := left.SortFields()
			rightName, rightYear, rightSortable := right.SortFields()
			if !leftSortable || !rightSortable {
				return false
			}
			switch m.sortMode {
			case sortNameAscending:
				return strings.ToLower(leftName) < strings.ToLower(rightName)
			case sortNameDescending:
				return strings.ToLower(leftName) > strings.ToLower(rightName)
			case sortYearAscending, sortYearDescending:
				if leftYear == 0 || rightYear == 0 {
					return rightYear == 0 && leftYear != 0
				}
				if leftYear == rightYear {
					return strings.ToLower(leftName) < strings.ToLower(rightName)
				}
				if m.sortMode == sortYearAscending {
					return leftYear < rightYear
				}
				return leftYear > rightYear
			}
			return false
		})
	}
	return result
}

func (m browserModel[T]) filteredLevel(level int) []indexed[T] {
	if level == len(m.levels)-1 {
		return m.filteredCurrent()
	}
	current := m.levels[level]
	items := filterItems(current.items, current.filter)
	if level != 0 || len(m.options.ParentGroups) == 0 {
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
		info, isStream := StreamInfo{}, false
		if ok {
			info, isStream = stream.StreamInfo()
		}
		if isStream && info.CacheApplicable && m.cachedOnly && !info.Cached {
			continue
		}
		if isStream && m.quality != 0 && info.Quality != m.quality {
			continue
		}
		result = append(result, value)
	}
	if m.streamSort != sortRelevance {
		sort.SliceStable(result, func(i, j int) bool {
			leftStream, leftOK := any(result[i].item).(streamItem)
			rightStream, rightOK := any(result[j].item).(streamItem)
			if !leftOK || !rightOK {
				return false
			}
			leftInfo, leftIsStream := leftStream.StreamInfo()
			rightInfo, rightIsStream := rightStream.StreamInfo()
			if !leftIsStream || !rightIsStream {
				return false
			}
			switch m.streamSort {
			case sortQualityAscending:
				return leftInfo.Quality < rightInfo.Quality
			case sortQualityDescending:
				return leftInfo.Quality > rightInfo.Quality
			case sortCachedFirst:
				return leftInfo.CacheApplicable && rightInfo.CacheApplicable && leftInfo.Cached && !rightInfo.Cached
			case sortUncachedFirst:
				return leftInfo.CacheApplicable && rightInfo.CacheApplicable && !leftInfo.Cached && rightInfo.Cached
			case sortNameAscending, sortNameDescending:
				leftSortable, leftOK := any(result[i].item).(sortableItem)
				rightSortable, rightOK := any(result[j].item).(sortableItem)
				if !leftOK || !rightOK {
					return false
				}
				leftName, _, _ := leftSortable.SortFields()
				rightName, _, _ := rightSortable.SortFields()
				if m.streamSort == sortNameAscending {
					return strings.ToLower(leftName) < strings.ToLower(rightName)
				}
				return strings.ToLower(leftName) > strings.ToLower(rightName)
			}
			return false
		})
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
	rows := max(1, height-6)
	current := m.levels[len(m.levels)-1]
	panes := make([]visiblePane[T], 0, len(m.levels)+1)
	for i, level := range m.levels {
		title := level.title
		if i == 0 && len(m.options.ParentGroups) > 0 {
			title = groupTabs(m.options.ParentGroups, m.groupIndex)
		}
		panes = append(panes, visiblePane[T]{title: title, items: m.filteredLevel(i), index: level.index, filter: level.filter, active: !m.focusRight && i == len(m.levels)-1})
	}
	rightTitle := m.right.title
	if m.rightHasStreams() {
		qualityLabel := "All qualities"
		if m.quality > 0 {
			qualityLabel = fmt.Sprintf("%dp", m.quality)
		}
		if m.rightCacheApplicable() {
			cacheLabel := "Cached"
			if !m.cachedOnly {
				cacheLabel = "All"
			}
			rightTitle = fmt.Sprintf("Streams  [%s]  [%s]", cacheLabel, qualityLabel)
		} else {
			rightTitle = fmt.Sprintf("Streams  [%s]", qualityLabel)
		}
	}
	if rightTitle != "" || len(m.right.items) > 0 || m.loading && !m.searching || m.err != nil {
		panes = append(panes, visiblePane[T]{title: rightTitle, items: m.filteredRight(), index: m.right.index, filter: m.right.filter, active: m.focusRight, loading: m.loading && !m.searching, err: m.err})
	}
	visible, widths := paneLayout(width, panes)
	rendered := make([]string, len(visible))
	for i, pane := range visible {
		rendered[i] = renderBrowserPane(pane.title, pane.items, pane.index, widths[i], rows, pane.active, pane.filter, pane.loading, pane.err, m.mode)
	}
	breadcrumb := m.breadcrumb()
	helpText := "? keys  m mode  s sort  h/l focus  j/k move  enter open  / filter  q quit"
	if m.options.Requery != nil {
		helpText = "? keys  m mode  s sort  h/l focus  j/k move  enter open  ctrl-h history  ctrl-p search  / filter  q quit"
	}
	if len(m.options.ParentGroups) > 1 {
		helpText = "tab movie/series  " + helpText
	}
	if m.focusRight {
		helpText = "h/l focus  j/k move  enter open/select  / filter  esc back"
		if m.rightHasStreams() && m.rightCacheApplicable() {
			helpText = "c cached/all  v quality  " + helpText
		}
	}
	if m.playing {
		helpText = "PLAYING  x stop  |  " + helpText
	}
	if m.options.ToggleHistory != nil && m.options.History != nil && len(m.levels) == 1 && !m.focusRight {
		helpText = "w history  " + helpText
	}
	if m.inHistoryRoot() && m.options.RemoveHistory != nil && m.options.History != nil {
		helpText = "d remove  " + helpText
	}
	base := ansi.Truncate(breadcrumb, width, "...") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, rendered...) + "\n" + hintStyle.Render(helpText) + "\n"
	var modal string
	switch {
	case m.helpMenu:
		modal = m.helpModal()
	case m.customPlayer:
		modal = inputModal("Custom player", m.customPlayerValue, "Enter save  Esc cancel")
	case m.settingsMenu:
		modal = m.settingsModal()
	case m.sortMenu:
		modal = sortModal(m.focusRight && m.rightHasStreams())
	case m.modeMenu:
		modal = modeModal(m.contextModes())
	case m.querying:
		modal = inputModal("Search", m.query, "Enter search  Esc cancel")
	case m.filtering:
		filter := current.filter
		if m.focusRight {
			filter = m.right.filter
		}
		modal = inputModal("Filter active pane", filter, "Enter apply  Esc clear")
	}
	view := base
	if modal != "" {
		view = overlay(view, modal, width)
	}
	if m.searching {
		view = overlay(view, activityModal(m.spinnerFrame, "Searching"), width)
	} else if m.historyBusy {
		view = overlay(view, activityModal(m.spinnerFrame, "Updating history"), width)
	}
	if m.notice != "" {
		view = toastOverlay(view, m.notice, width)
	}
	return "\x1b]0;" + plainLabel(breadcrumb) + "\x07" + view
}

func paneLayout[T item](width int, panes []visiblePane[T]) ([]visiblePane[T], []int) {
	if len(panes) == 0 {
		return nil, nil
	}
	count := min(3, len(panes))
	if width < 64 {
		count = 1
	} else if width < 88 {
		count = min(2, count)
	}
	if count == 1 {
		active := len(panes) - 1
		for i := range panes {
			if panes[i].active {
				active = i
				break
			}
		}
		return panes[active : active+1], []int{max(18, width-2)}
	}
	visible := panes[len(panes)-count:]
	minimums := []int{24, 40}
	weights := []int{1, 2}
	if count == 3 {
		minimums = []int{24, 24, 40}
		weights = []int{1, 1, 2}
	}
	extra := max(0, width-sum(minimums))
	weightTotal := sum(weights)
	widths := make([]int, count)
	used := 0
	for i := range count {
		share := extra * weights[i] / weightTotal
		widths[i] = minimums[i] + share - 2
		used += share
	}
	widths[count-1] += extra - used
	return visible, widths
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func (m browserModel[T]) breadcrumb() string {
	parts := make([]string, 0, len(m.crumbs)+2)
	if len(m.options.ParentGroups) > 0 && m.groupIndex >= 0 && m.groupIndex < len(m.options.ParentGroups) {
		group := m.options.ParentGroups[m.groupIndex]
		if group != "" {
			parts = append(parts, strings.ToUpper(group[:1])+group[1:])
		}
	}
	if m.activeQuery != "" {
		parts = append(parts, m.activeQuery)
	} else if len(parts) == 0 {
		parts = append(parts, "Search")
	}
	parts = append(parts, m.crumbs...)
	return strings.Join(parts, " / ")
}

func sortModal(torrents bool) string {
	lines := []string{
		headerStyle.Render("Sort results"),
		"a   Name ascending",
		"A   Name descending",
		"y   Year ascending",
		"Y   Year descending",
		"d/r Default relevance",
	}
	if torrents {
		lines = []string{
			headerStyle.Render("Sort streams"),
			"q   Quality ascending",
			"Q   Quality descending",
			"c   Cached first",
			"C   Uncached first",
			"n   Name ascending",
			"N   Name descending",
			"d/r Default ranking",
		}
	}
	lines = append(lines, "", hintStyle.Render("Esc cancel"))
	return activeBorder.Padding(0, 2).Render(strings.Join(lines, "\n"))
}

func inputModal(title, value, help string) string {
	input := ansi.Truncate(value, 48, "...") + "_"
	return activeBorder.Width(50).Padding(0, 1).Render(strings.Join([]string{
		headerStyle.Render(title),
		input,
		"",
		hintStyle.Render(help + "  Ctrl-W word  Ctrl-U line"),
	}, "\n"))
}

func activityModal(frame int, label string) string {
	return activeBorder.Padding(0, 2).Render(spinnerFrames[frame%len(spinnerFrames)] + " " + label)
}

func (m browserModel[T]) helpModal() string {
	bindings := m.filteredHelpBindings()
	lines := []string{headerStyle.Render("Keybindings"), "Search: " + m.helpFilter + "_", ""}
	if len(bindings) == 0 {
		lines = append(lines, "No matching commands")
	} else {
		selected := clamp(m.helpIndex, len(bindings))
		height := m.height
		if height <= 0 {
			height = 24
		}
		visible := max(1, height-7)
		start := max(0, min(selected-visible/2, len(bindings)-visible))
		end := min(len(bindings), start+visible)
		lines[0] += hintStyle.Render(fmt.Sprintf("  %d-%d/%d", start+1, end, len(bindings)))
		for i := start; i < end; i++ {
			binding := bindings[i]
			line := fmt.Sprintf("%-20s %s", binding.keys, binding.label)
			if i == selected {
				line = selectedStyle.Width(50).Render("> " + line)
			} else {
				line = "  " + line
			}
			lines = append(lines, line)
		}
	}
	lines = append(lines, "", hintStyle.Render("Type to filter  Up/Down select  Enter run  Esc close"))
	return activeBorder.Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func (m browserModel[T]) settingsModal() string {
	group := "Movie"
	if len(m.options.ParentGroups) > 0 {
		group = strings.ToUpper(m.options.ParentGroups[m.groupIndex][:1]) + m.options.ParentGroups[m.groupIndex][1:]
	}
	quality := "All"
	if m.quality != 0 {
		quality = fmt.Sprintf("%dp", m.quality)
	}
	cached := "Cached only"
	if !m.cachedOnly {
		cached = "All streams"
	}
	player := m.player
	if player == "" {
		player = "System default"
	}
	provider := m.provider
	if provider == "" {
		provider = "Default"
	}
	values := []string{group, quality, cached, provider, player}
	labels := []string{"Media type", "Quality", "Availability", "Provider", "Player"}
	for _, modeGroup := range settingModeGroups {
		modes := m.options.ModeOptions[modeGroup]
		value := "Default"
		key := m.mode[modeGroup]
		for i, mode := range modes {
			if key == mode.Key || key == "" && i == 0 {
				value = mode.Name
				break
			}
		}
		labels = append(labels, strings.ToUpper(modeGroup[:1])+modeGroup[1:]+" detail")
		values = append(values, value)
	}
	lines := []string{headerStyle.Render("Settings"), ""}
	for i := range labels {
		line := fmt.Sprintf("%-20s  < %-16s >", labels[i], values[i])
		if i == m.settingsIndex {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", hintStyle.Render("Up/Down item  Left/Right change  Enter edit player  Esc close"))
	return activeBorder.Padding(0, 1).Render(strings.Join(lines, "\n"))
}

func overlay(base, modal string, width int) string {
	baseLines := strings.Split(strings.TrimSuffix(base, "\n"), "\n")
	modalLines := strings.Split(modal, "\n")
	modalWidth := lipgloss.Width(modal)
	x := max(0, (width-modalWidth)/2)
	y := max(0, (len(baseLines)-len(modalLines))/2)
	return overlayAt(baseLines, modalLines, width, x, y)
}

func toastOverlay(base, message string, width int) string {
	contentWidth := max(10, min(48, width-6))
	toast := toastBorder.Render(ansi.Truncate(plainLabel(message), contentWidth, "..."))
	baseLines := strings.Split(strings.TrimSuffix(base, "\n"), "\n")
	toastLines := strings.Split(toast, "\n")
	x := max(0, width-lipgloss.Width(toast)-1)
	y := max(0, len(baseLines)-len(toastLines)-1)
	return overlayAt(baseLines, toastLines, width, x, y)
}

func overlayAt(baseLines, modalLines []string, width, x, y int) string {
	modalWidth := 0
	for _, line := range modalLines {
		modalWidth = max(modalWidth, lipgloss.Width(line))
	}
	for len(baseLines) < y+len(modalLines) {
		baseLines = append(baseLines, "")
	}
	for i, modalLine := range modalLines {
		baseLine := baseLines[y+i]
		left := ansi.Cut(baseLine, 0, x)
		if padding := x - lipgloss.Width(left); padding > 0 {
			left += strings.Repeat(" ", padding)
		}
		right := ansi.Cut(baseLine, x+modalWidth, width)
		baseLines[y+i] = left + modalLine + right
	}
	return strings.Join(baseLines, "\n") + "\n"
}

func (m browserModel[T]) rightHasStreams() bool {
	for _, value := range m.right.items {
		if stream, ok := any(value).(streamItem); ok {
			_, isStream := stream.StreamInfo()
			if isStream {
				return true
			}
		}
	}
	return false
}

func (m browserModel[T]) rightCacheApplicable() bool {
	for _, value := range m.right.items {
		if stream, ok := any(value).(streamItem); ok {
			info, isStream := stream.StreamInfo()
			if isStream && info.CacheApplicable {
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

func renderBrowserPane[T item](title string, items []indexed[T], selected, width, rows int, active bool, filter string, loading bool, loadErr error, selectedModes map[string]string) string {
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
			label := plainLabel(items[i].item.Label())
			context := ""
			if contextual, ok := any(items[i].item).(contextualItem); ok {
				modes := contextual.ContextModes()
				if len(modes) > 0 {
					group := modes[0].Group
					if group == "" {
						group = modes[0].Name
					}
					key := selectedModes[group]
					if key == "" {
						key = modes[0].Key
					}
					matched := false
					for _, mode := range modes {
						if mode.Key == key {
							context = plainLabel(mode.Value)
							matched = true
							break
						}
					}
					if !matched {
						context = plainLabel(modes[0].Value)
					}
				}
			}
			available := max(1, contentWidth-2)
			unavailable, isUnavailable := any(items[i].item).(unavailableItem)
			if context != "" {
				context = ansi.Truncate(context, max(1, available/2), "...")
				label = ansi.Truncate(label, max(1, available-lipgloss.Width(context)-1), "...")
				if isUnavailable && unavailable.Unavailable() {
					label = unavailableStyle.Render(label)
				}
				label += strings.Repeat(" ", max(1, available-lipgloss.Width(label)-lipgloss.Width(context))) + hintStyle.Render(context)
			} else {
				label = ansi.Truncate(label, available, "...")
				if isUnavailable && unavailable.Unavailable() {
					label = unavailableStyle.Render(label)
				}
			}
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
		if label == "Movie" {
			label = "Movies"
		}
		if i == active {
			label = headerStyle.Render("● " + label)
		} else {
			label = hintStyle.Render("○ " + label)
		}
		labels[i] = label
	}
	return strings.Join(labels, "    ")
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
	if len(items) == 0 && options.Play == nil {
		return zero, errors.New("no choices")
	}
	title := options.InitialTitle
	if title == "" {
		title = "Search results"
	}
	groupIndex := preferredGroupIndex(options.ParentGroups, options.PreferredGroup)
	cachedOnly := true
	if options.PreferredCached != nil {
		cachedOnly = *options.PreferredCached
	}
	initial := browserModel[T]{ctx: ctx, levels: []pane[T]{{title: title, items: items}}, load: load, options: options, groupIndex: groupIndex, cachedOnly: cachedOnly, quality: options.PreferredQuality, mode: options.PreferredModes, provider: options.PreferredProvider, player: options.PreferredPlayer, activeQuery: options.InitialQuery, width: 100, height: 24}
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
