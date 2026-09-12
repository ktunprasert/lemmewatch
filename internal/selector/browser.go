package selector

import (
	"context"
	"errors"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/help"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type BrowserOptions[T item] struct {
	InitialTitle        string
	InitialQuery        string
	InitialSearch       bool
	Version             string
	ParentGroups        []string
	PreferredGroup      string
	PreferredQuality    int
	PreferredCached     *bool
	PreferredProvider   string
	PreferredPlayer     string
	Providers           []string
	PreferredModes      map[string]string
	ModeOptions         map[string][]ContextMode
	SaveGroup           func(string) error
	SaveQuality         func(int) error
	SaveCached          func(bool) error
	SaveProvider        func(string) error
	ProviderNeedsAPIKey func(string) bool
	SaveProviderAPIKey  func(string, string) error
	SavePlayer          func(string) error
	SaveMode            func(string, string) error
	ChildTitle          func(T) string
	Refresh             func(context.Context, T) ([]T, error)
	Play                func(context.Context, T) error
	Progress            func() <-chan string
	Requery             func(context.Context, string) ([]T, error)
	History             func(context.Context) ([]T, error)
	Watched             map[string]bool
	ToggleWatched       func(context.Context, T) (map[string]bool, error)
	RemoveHistory       func(context.Context, T) error
	SearchGroups        []string
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
	SortFields() (name string, year int, playedAt time.Time, ok bool)
}

type sortMode int

const (
	sortRelevance sortMode = iota
	sortNameAscending
	sortNameDescending
	sortYearAscending
	sortYearDescending
	sortPlayedAscending
	sortPlayedDescending
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
	items        []T
	watched      map[string]bool
	watchChanged bool
	err          error
}
type episodeSwitched[T item] struct {
	episodes     []T
	streams      []T
	seasonIndex  int
	episodeIndex int
	seasonLabel  string
	episodeLabel string
	title        string
	key          string
	provider     string
	loadID       uint64
	direction    int
	found        bool
	err          error
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
type watchableItem interface{ WatchIdentity() (string, []string) }
type statusItem interface{ Status(map[string]bool) string }

func isWatched(value any, state map[string]bool) bool {
	if terminal, ok := value.(terminalItem); ok && terminal.Terminal() {
		return false
	}
	watchable, ok := value.(watchableItem)
	if !ok {
		return false
	}
	identity, keys := watchable.WatchIdentity()
	if identity == "" {
		return false
	}
	if len(keys) == 0 {
		return state[identity]
	}
	for _, key := range keys {
		if !state[identity+":"+key] {
			return false
		}
	}
	return true
}

type browserModel[T item] struct {
	ctx                 context.Context
	levels              []pane[T]
	right               pane[T]
	load                func(context.Context, T) ([]T, error)
	options             BrowserOptions[T]
	crumbs              []string
	groupIndex          int
	focusRight          bool
	loading             bool
	searching           bool
	historyBusy         bool
	err                 error
	query               string
	activeQuery         string
	mode                map[string]string
	sortMode            sortMode
	streamSort          sortMode
	helpFilter          string
	helpIndex           int
	settingsIndex       int
	player              string
	provider            string
	customPlayerValue   string
	providerAPIKeyFor   string
	providerAPIKeyValue string
	pendingG            bool
	cachedOnly          bool
	quality             int
	overlay             overlayKind
	overlayStack        []overlayKind
	width               int
	height              int
	chosen              bool
	choice              T
	playback            playbackModel
	loadCache           map[string][]T
	loadID              uint64
	help                help.Model
	toasts              toastModel
}

var (
	activeBorder     = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(accentColor)
	inactiveBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#A0A0A0", Dark: "#555555"})
	toastBorder      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.AdaptiveColor{Light: "#B42318", Dark: "#FF6B6B"}).Padding(0, 1)
	unavailableStyle = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#8A8A8A", Dark: "#666666"})
)

func (m browserModel[T]) Init() tea.Cmd {
	if !m.options.InitialSearch || m.options.Requery == nil {
		return nil
	}
	query := m.options.InitialQuery
	return func() tea.Msg {
		items, err := m.options.Requery(m.ctx, query)
		return requeryFinished[T]{items: items, query: query, err: err}
	}
}

func (m browserModel[T]) Update(message tea.Msg) (result tea.Model, command tea.Cmd) {
	entryGen := m.toasts.gen
	defer func() {
		updated, ok := result.(browserModel[T])
		if !ok {
			return
		}
		if updated.toasts.gen != entryGen {
			if cmd := updated.toasts.expiryCmd(); cmd != nil && command == nil {
				command = cmd
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
			m.toasts.Err(ToastLoad, "Load failed: %s", msg.err.Error())
		}
	case playProgress:
		return m.updatePlaybackProgress(msg)
	case playFinished:
		return m.updatePlaybackFinished(msg)
	case requeryFinished[T]:
		m.loading = false
		m.searching = false
		if msg.err != nil {
			m.toasts.Err(ToastSearch, "Search failed: %s", msg.err.Error())
			break
		}
		title := "Search results"
		m.levels = []pane[T]{{title: title, items: msg.items}}
		m.options.ParentGroups = m.options.SearchGroups
		m.right = pane[T]{}
		m.crumbs = nil
		m.activeQuery = msg.query
		m.focusRight = false
		m.toasts.Clear()
	case historyFinished[T]:
		m.loading = false
		if msg.err != nil {
			m.toasts.Err(ToastHistory, "History failed: %s", msg.err.Error())
			break
		}
		m.levels = []pane[T]{{title: "History", items: msg.items}}
		m.right = pane[T]{}
		m.crumbs = nil
		m.options.ParentGroups = nil
		m.activeQuery = "History"
		m.focusRight = false
		m.toasts.Clear()
	case historyChanged[T]:
		m.historyBusy = false
		if msg.err != nil {
			m.toasts.Err(ToastHistory, "History update failed: %s", msg.err.Error())
			break
		}
		if m.inHistoryRoot() {
			m.current().items = msg.items
			m.current().index = clamp(m.current().index, len(msg.items))
		}
		if msg.watchChanged {
			m.options.Watched = msg.watched
			m.toasts.Set(ToastHistory, "Watched state updated")
		} else {
			m.toasts.Set(ToastHistory, "Removed from history")
		}
	case episodeSwitched[T]:
		if msg.provider != m.provider || msg.loadID != m.loadID {
			break
		}
		m.loading = false
		if msg.err != nil {
			m.toasts.Err(ToastLoad, "Load failed: %s", msg.err.Error())
			break
		}
		if !msg.found {
			m.toasts.Set(ToastEpisode, "%s", episodeBoundaryNotice(msg.direction))
			break
		}
		seasonLevel := len(m.levels) - 2
		m.levels[seasonLevel].index = msg.seasonIndex
		m.levels[seasonLevel].filter = ""
		m.current().items = msg.episodes
		m.current().index = msg.episodeIndex
		m.current().filter = ""
		m.right = pane[T]{title: msg.title, items: msg.streams}
		m.err = nil
		m.focusRight = true
		if seasonLevel+1 < len(m.crumbs) {
			m.crumbs[seasonLevel] = msg.seasonLabel
			m.crumbs[seasonLevel+1] = msg.episodeLabel
		}
		if msg.key != "" {
			if m.loadCache == nil {
				m.loadCache = make(map[string][]T)
			}
			m.loadCache[msg.key] = msg.streams
		}
	case spinnerTick:
		if m.searching || m.historyBusy || m.toasts.spinning {
			m.toasts.frame++
			return m, spinnerCommand()
		}
	case toastExpired:
		m.toasts.Expired(msg.id)
	case tea.KeyMsg:
		if m.searching || m.historyBusy {
			if msg.String() == "ctrl+c" {
				return m, tea.Quit
			}
			return m, nil
		}
		if m.overlay != overlayNone {
			return m.updateOverlay(msg)
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
			m.playback.stopPlayback()
			return m, tea.Quit
		case "x":
			if m.playback.stopPlayback() {
				m.toasts.Set(ToastPlayback, "Stopping playback...")
			}
		case "g":
			m.pendingG = true
		case "G":
			m.move(1 << 30)
		case "r", "f5":
			if m.canRefresh() {
				items := m.filteredCurrent()
				return m.loadSelected(items[m.current().index].item, true)
			}
		case "n":
			if m.canSwitchEpisode() {
				return m.switchEpisode(1)
			}
		case "p":
			if m.canSwitchEpisode() {
				return m.switchEpisode(-1)
			}
		case "s":
			if (!m.focusRight && len(m.levels) == 1) || (m.focusRight && m.rightHasStreams()) {
				m.openOverlay(overlaySort)
			}
		case "m":
			if len(m.contextModes()) > 0 {
				m.openOverlay(overlayMode)
			}
		case "/":
			m.openOverlay(overlayFilter)
		case "?":
			m.openOverlay(overlayHelp)
			m.helpFilter = ""
			m.helpIndex = 0
		case ";":
			m.openOverlay(overlaySettings)
			m.settingsIndex = 0
		case "ctrl+p":
			if m.options.Requery != nil && !m.loading {
				m.openOverlay(overlayQuery)
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
			if selected, ok := m.selectedWatchable(); ok && m.options.ToggleWatched != nil {
				m.historyBusy = true
				return m, tea.Batch(func() tea.Msg {
					watched, err := m.options.ToggleWatched(m.ctx, selected)
					return historyChanged[T]{watched: watched, watchChanged: true, err: err}
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
				m.toasts.Clear()
				if m.options.SaveGroup != nil {
					if err := m.options.SaveGroup(m.options.ParentGroups[m.groupIndex]); err != nil {
						m.toasts.Err(ToastSettings, "Could not save media tab preference")
					}
				}
			}
		case "c":
			if m.focusRight && m.rightHasStreams() && m.rightCacheApplicable() {
				m.cachedOnly = !m.cachedOnly
				m.right.index = 0
				m.toasts.Clear()
			}
		case "v":
			if m.focusRight && m.rightHasStreams() {
				m.quality = nextQuality(m.quality)
				m.right.index = 0
				m.toasts.Clear()
				if m.options.SaveQuality != nil {
					if err := m.options.SaveQuality(m.quality); err != nil {
						m.toasts.Err(ToastSettings, "Could not save quality preference")
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
		case "H":
			for m.focusRight || len(m.levels) > 1 {
				m.back()
			}
			m.crumbs = nil
		case "right", "l":
			if !m.loading {
				return m.confirm()
			}
		case "up", "k":
			m.move(-1)
		case "down", "j":
			m.move(1)
		case "pgup":
			m.page(-m.pageSize())
		case "pgdown":
			m.page(m.pageSize())
		case "ctrl+d":
			m.page(max(1, m.pageSize()/2))
		case "ctrl+u":
			m.page(-max(1, m.pageSize()/2))
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
				if isStream && !info.Playable && !info.CacheApplicable {
					m.toasts.Err(ToastStream, "Stream is not playable")
					return m, nil
				}
			}
			if m.options.Play == nil {
				m.choice = selected
				m.chosen = true
				return m, tea.Quit
			}
			if m.playback.busy() {
				m.toasts.Set(ToastPlayback, "Stop current playback before starting another")
				return m, nil
			}
			if watchable, ok := any(selected).(watchableItem); ok {
				identity, keys := watchable.WatchIdentity()
				if identity != "" {
					if m.options.Watched == nil {
						m.options.Watched = make(map[string]bool)
					}
					m.options.Watched[identity] = true
					for _, key := range keys {
						m.options.Watched[identity+":"+key] = true
					}
				}
			}
			return m.startPlayback(selected)
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
	m.toasts.Clear()
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
	load := m.load
	if refresh && m.options.Refresh != nil {
		load = m.options.Refresh
	}
	return m, func() tea.Msg {
		items, err := load(m.ctx, selected)
		return loaded[T]{items: items, err: err, key: key, provider: m.provider, loadID: loadID}
	}
}

func (m browserModel[T]) canRefresh() bool {
	if m.loading {
		return false
	}
	items := m.filteredCurrent()
	if len(items) == 0 {
		return false
	}
	cacheable, ok := any(items[m.current().index].item).(cacheableItem)
	return ok && cacheable.CacheKey() != ""
}

func (m browserModel[T]) switchEpisode(direction int) (tea.Model, tea.Cmd) {
	if m.loading || len(m.levels) < 2 {
		return m, nil
	}
	visibleEpisodes := m.filteredCurrent()
	if len(visibleEpisodes) == 0 {
		return m, nil
	}
	currentEpisode := visibleEpisodes[clamp(m.current().index, len(visibleEpisodes))]
	current := currentEpisode.item
	if cacheable, ok := any(current).(cacheableItem); !ok || cacheable.CacheKey() == "" {
		return m, nil
	}
	episodes := m.current().items
	for index := currentEpisode.index + direction; index >= 0 && index < len(episodes); index += direction {
		if itemUnavailable(episodes[index]) {
			if direction > 0 {
				m.toasts.Set(ToastEpisode, "%s", episodeBoundaryNotice(direction))
				return m, nil
			}
			continue
		}
		m.current().filter = ""
		m.current().index = index
		if len(m.crumbs) > 0 {
			m.crumbs[len(m.crumbs)-1] = plainLabel(episodes[index].Label())
		}
		return m.loadSelected(episodes[index], false)
	}

	seasonLevel := len(m.levels) - 2
	visibleSeasons := m.filteredLevel(seasonLevel)
	if len(visibleSeasons) == 0 {
		return m, nil
	}
	currentSeason := visibleSeasons[clamp(m.levels[seasonLevel].index, len(visibleSeasons))]
	seasons := m.levels[seasonLevel].items
	seasonIndex := currentSeason.index + direction
	if seasonIndex < 0 || seasonIndex >= len(seasons) {
		m.toasts.Set(ToastEpisode, "%s", episodeBoundaryNotice(direction))
		return m, nil
	}

	m.loading = true
	m.toasts.Clear()
	m.loadID++
	loadID := m.loadID
	provider := m.provider
	loadCache := m.loadCache
	return m, func() tea.Msg {
		for index := seasonIndex; index >= 0 && index < len(seasons); index += direction {
			season := seasons[index]
			episodes, err := m.load(m.ctx, season)
			if err != nil {
				return episodeSwitched[T]{provider: provider, loadID: loadID, direction: direction, err: err}
			}
			start, end, step := 0, len(episodes), 1
			if direction < 0 {
				start, end, step = len(episodes)-1, -1, -1
			}
			for episodeIndex := start; episodeIndex != end; episodeIndex += step {
				episode := episodes[episodeIndex]
				if itemUnavailable(episode) {
					if direction > 0 {
						return episodeSwitched[T]{provider: provider, loadID: loadID, direction: direction}
					}
					continue
				}
				key := itemCacheKey(episode, provider)
				streams, cached := loadCache[key]
				if !cached {
					streams, err = m.load(m.ctx, episode)
					if err != nil {
						return episodeSwitched[T]{provider: provider, loadID: loadID, direction: direction, err: err}
					}
				}
				return episodeSwitched[T]{episodes: episodes, streams: streams, seasonIndex: index, episodeIndex: episodeIndex, seasonLabel: plainLabel(season.Label()), episodeLabel: plainLabel(episode.Label()), title: m.childTitle(episode), key: key, provider: provider, loadID: loadID, direction: direction, found: true}
			}
		}
		return episodeSwitched[T]{provider: provider, loadID: loadID, direction: direction}
	}
}

func (m browserModel[T]) canSwitchEpisode() bool {
	if !m.focusRight || !m.rightHasStreams() || len(m.levels) < 2 {
		return false
	}
	episodes := m.filteredCurrent()
	if len(episodes) == 0 {
		return false
	}
	cacheable, ok := any(episodes[clamp(m.current().index, len(episodes))].item).(cacheableItem)
	return ok && cacheable.CacheKey() != ""
}

func itemUnavailable[T item](value T) bool {
	unavailable, ok := any(value).(unavailableItem)
	return ok && unavailable.Unavailable()
}

func itemCacheKey[T item](value T, provider string) string {
	cacheable, ok := any(value).(cacheableItem)
	if !ok || cacheable.CacheKey() == "" {
		return ""
	}
	key := cacheable.CacheKey()
	if provider != "" {
		key = provider + ":" + key
	}
	return key
}

func episodeBoundaryNotice(direction int) string {
	if direction < 0 {
		return "No previous episode"
	}
	return "No next aired episode"
}

func (m *browserModel[T]) back() {
	if m.loading {
		m.loading = false
		m.loadID++
	}
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
	m.toasts.Clear()
}

func (m *browserModel[T]) move(delta int) {
	if m.focusRight {
		m.right.index = clamp(m.right.index+delta, len(m.filteredRight()))
	} else {
		m.current().index = clamp(m.current().index+delta, len(m.filteredCurrent()))
	}
}

func (m *browserModel[T]) page(delta int) {
	if m.focusRight {
		items := m.filteredRight()
		first, last := availableBounds(items)
		m.right.index = boundedPage(m.right.index, delta, len(items), first, last)
	} else {
		items := m.filteredCurrent()
		first, last := availableBounds(items)
		m.current().index = boundedPage(m.current().index, delta, len(items), first, last)
	}
}

func availableBounds[T item](items []indexed[T]) (first, last int) {
	first, last = -1, -1
	for i, value := range items {
		if !itemUnavailable(value.item) {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return first, last
}

func boundedPage(index, delta, length, first, last int) int {
	target := clamp(index+delta, length)
	if delta > 0 && last >= 0 && index < last && target > last {
		return last
	}
	if delta < 0 && first >= 0 && index > first && target < first {
		return first
	}
	return target
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

func (m browserModel[T]) selectedWatchable() (T, bool) {
	var zero T
	if m.focusRight {
		if m.rightHasStreams() {
			return zero, false
		}
		items := m.filteredRight()
		if len(items) == 0 {
			return zero, false
		}
		selected := items[clamp(m.right.index, len(items))].item
		_, ok := any(selected).(watchableItem)
		return selected, ok
	}
	items := m.filteredCurrent()
	if len(items) == 0 {
		return zero, false
	}
	selected := items[clamp(m.current().index, len(items))].item
	_, ok := any(selected).(watchableItem)
	return selected, ok
}

func (m *browserModel[T]) current() *pane[T] { return &m.levels[len(m.levels)-1] }
func (m browserModel[T]) pageSize() int      { return max(1, m.height-6) }

func (m browserModel[T]) filteredCurrent() []indexed[T] {
	current := m.levels[len(m.levels)-1]
	items := filterItems(current.items, current.filter)
	if len(m.levels) != 1 {
		return items
	}
	result := items
	if len(m.options.ParentGroups) > 0 {
		group := m.options.ParentGroups[m.groupIndex]
		result = items[:0]
		for _, value := range items {
			if grouped, ok := any(value.item).(groupedItem); ok && grouped.Group() == group {
				result = append(result, value)
			}
		}
	}
	if m.sortMode != sortRelevance {
		sort.SliceStable(result, func(i, j int) bool {
			left, leftOK := any(result[i].item).(sortableItem)
			right, rightOK := any(result[j].item).(sortableItem)
			if !leftOK || !rightOK {
				return false
			}
			leftName, leftYear, leftPlayed, leftSortable := left.SortFields()
			rightName, rightYear, rightPlayed, rightSortable := right.SortFields()
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
			case sortPlayedAscending, sortPlayedDescending:
				if leftPlayed.IsZero() || rightPlayed.IsZero() {
					return rightPlayed.IsZero() && !leftPlayed.IsZero()
				}
				if leftPlayed.Equal(rightPlayed) {
					return strings.ToLower(leftName) < strings.ToLower(rightName)
				}
				if m.sortMode == sortPlayedAscending {
					return leftPlayed.Before(rightPlayed)
				}
				return leftPlayed.After(rightPlayed)
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
				leftName, _, _, _ := leftSortable.SortFields()
				rightName, _, _, _ := rightSortable.SortFields()
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
	initial := browserModel[T]{ctx: ctx, levels: []pane[T]{{title: title, items: items}}, load: load, options: options, groupIndex: groupIndex, cachedOnly: cachedOnly, quality: options.PreferredQuality, mode: options.PreferredModes, provider: options.PreferredProvider, player: options.PreferredPlayer, activeQuery: options.InitialQuery, searching: options.InitialSearch, loading: options.InitialSearch, width: 100, height: 24, help: newHelpModel()}
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
