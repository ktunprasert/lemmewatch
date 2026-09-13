package selector

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type overlayKind int

const (
	overlayNone overlayKind = iota
	overlayHelp
	overlaySettings
	overlaySort
	overlayMode
	overlayCustomPlayer
	overlayProviderAPIKey
	overlayQuery
	overlayFilter
	overlayPaneSizes
)

func (m *browserModel[T]) openOverlay(kind overlayKind) {
	if m.overlay != overlayNone {
		m.overlayStack = append(m.overlayStack, m.overlay)
	}
	m.overlay = kind
}

func (m *browserModel[T]) closeOverlay() {
	if n := len(m.overlayStack); n > 0 {
		m.overlay = m.overlayStack[n-1]
		m.overlayStack = m.overlayStack[:n-1]
		return
	}
	m.overlay = overlayNone
}

func (m *browserModel[T]) closeAllOverlays() {
	m.overlay = overlayNone
	m.overlayStack = nil
}

func (m *browserModel[T]) updateOverlay(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.overlay {
	case overlayHelp:
		return m.updateHelp(msg)
	case overlaySettings:
		return m.updateSettings(msg)
	case overlaySort:
		return m.updateSort(msg)
	case overlayMode:
		return m.updateMode(msg)
	case overlayCustomPlayer:
		return m.updateCustomPlayer(msg)
	case overlayProviderAPIKey:
		return m.updateProviderAPIKey(msg)
	case overlayQuery:
		return m.updateQuery(msg)
	case overlayFilter:
		return m.updateFilter(msg)
	case overlayPaneSizes:
		return m.updatePaneSizes(msg)
	default:
		return m, nil
	}
}

func (m browserModel[T]) updateMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	m.closeAllOverlays()
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
	lines := make([]string, 0, len(modes)+2)
	for _, mode := range modes {
		lines = append(lines, fmt.Sprintf("[%s] %s", mode.Key, mode.Name))
	}
	lines = append(lines, "", renderHelpLine(newHelpModel(), 32, []key.Binding{
		hintBinding("esc", "cancel"),
	}, helpLineOptions{}))
	return titledModal("Mode", strings.Join(lines, "\n"), activeBorder.Padding(0, 1))
}

func (m browserModel[T]) updateHelp(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	bindings := m.filteredHelpBindings()
	switch msg.String() {
	case "esc":
		m.closeAllOverlays()
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
		m.closeAllOverlays()
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

const settingsCount = 11

func (m browserModel[T]) updateSettings(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc", ";":
		m.closeAllOverlays()
	case "up", "k":
		m.settingsIndex = clamp(m.settingsIndex-1, settingsCount)
	case "down", "j":
		m.settingsIndex = clamp(m.settingsIndex+1, settingsCount)
	case "left", "h":
		m.changeSetting(-1)
	case "right", "l":
		m.changeSetting(1)
	case "enter":
		if m.settingsIndex == 4 {
			m.openOverlay(overlayCustomPlayer)
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
		selected := m.options.Providers[wrapIndex(index+delta, len(m.options.Providers))]
		if m.options.ProviderNeedsAPIKey != nil && m.options.ProviderNeedsAPIKey(selected) {
			m.openOverlay(overlayProviderAPIKey)
			m.providerAPIKeyFor = selected
			m.providerAPIKeyValue = ""
			m.toasts.Clear()
			return
		}
		m.selectProvider(selected, true)
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
	case 9, 10:
		m.paneSizeCount = m.settingsIndex - 7
		m.paneSizeValue = formatPaneSizes(paneSizeWeights(m.paneSizeCount, m.paneSizes))
		m.openOverlay(overlayPaneSizes)
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
		m.toasts.Err(ToastSettings, "Could not save preference")
	}
}

func (m *browserModel[T]) selectProvider(selected string, save bool) {
	m.provider = selected
	m.right = pane[T]{}
	m.focusRight = false
	m.loading = false
	m.loadCache = nil
	m.loadID++
	if save && m.options.SaveProvider != nil {
		m.saveSetting(m.options.SaveProvider(selected))
	}
}

func (m browserModel[T]) updateProviderAPIKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeOverlay()
		m.providerAPIKeyFor = ""
		m.providerAPIKeyValue = ""
	case "enter":
		key := strings.TrimSpace(m.providerAPIKeyValue)
		if key == "" {
			m.toasts.Err(ToastSettings, "API key is required")
			return m, nil
		}
		if m.options.SaveProviderAPIKey == nil {
			m.toasts.Err(ToastSettings, "Could not save API key")
			return m, nil
		}
		if err := m.options.SaveProviderAPIKey(m.providerAPIKeyFor, key); err != nil {
			m.toasts.Err(ToastSettings, "Could not save API key")
			return m, nil
		}
		selected := m.providerAPIKeyFor
		m.closeOverlay()
		m.providerAPIKeyFor = ""
		m.providerAPIKeyValue = ""
		m.toasts.Clear()
		m.selectProvider(selected, false)
	case "backspace", "ctrl+h":
		if len(m.providerAPIKeyValue) > 0 {
			runes := []rune(m.providerAPIKeyValue)
			m.providerAPIKeyValue = string(runes[:len(runes)-1])
		}
	case "ctrl+w":
		m.providerAPIKeyValue = strings.TrimRight(m.providerAPIKeyValue, " ")
		if end := strings.LastIndex(m.providerAPIKeyValue, " "); end >= 0 {
			m.providerAPIKeyValue = strings.TrimRight(m.providerAPIKeyValue[:end+1], " ")
		} else {
			m.providerAPIKeyValue = ""
		}
	case "ctrl+u":
		m.providerAPIKeyValue = ""
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeySpace {
			m.providerAPIKeyValue += " "
		} else if msg.Type == tea.KeyRunes {
			m.providerAPIKeyValue += string(msg.Runes)
		}
	}
	return m, nil
}

func (m browserModel[T]) updateCustomPlayer(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeOverlay()
	case "enter":
		m.player = strings.TrimSpace(m.customPlayerValue)
		m.closeOverlay()
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
		{keys: "H", label: "Go Home", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'H'}}},
		{keys: "Up / k", label: "Move up", key: tea.KeyMsg{Type: tea.KeyUp}},
		{keys: "Down / j", label: "Move down", key: tea.KeyMsg{Type: tea.KeyDown}},
		{keys: "PgUp", label: "Previous page", key: tea.KeyMsg{Type: tea.KeyPgUp}},
		{keys: "PgDown", label: "Next page", key: tea.KeyMsg{Type: tea.KeyPgDown}},
		{keys: "Ctrl-D", label: "Move half-page down", key: tea.KeyMsg{Type: tea.KeyCtrlD}},
		{keys: "Ctrl-U", label: "Move half-page up", key: tea.KeyMsg{Type: tea.KeyCtrlU}},
		{keys: "gg", label: "Move to first item", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g', 'g'}}},
		{keys: "G", label: "Move to last item", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}}},
		{keys: "/", label: "Filter active pane", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'/'}}},
		{keys: "s", label: "Sort active results", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}}},
		{keys: "m", label: "Choose detail mode", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}}},
		{keys: "i", label: "Toggle active pane info", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}}},
		{keys: "x", label: "Stop playback", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}},
		{keys: "c", label: "Toggle cached or all", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'c'}}},
		{keys: "v", label: "Cycle video quality", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}}},
		{keys: "?", label: "Show keybindings", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}}},
		{keys: ";", label: "Settings", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{';'}}},
		{keys: "q", label: "Quit", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'q'}}},
	}
	if m.canRefresh() {
		bindings = append(bindings, helpBinding{keys: "r / F5", label: "Refresh selected data", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}}})
	}
	if m.info[m.activeInfoKey()].open {
		bindings = append(bindings,
			helpBinding{keys: "Alt-j", label: "Scroll info down", key: tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{'j'}}},
			helpBinding{keys: "Alt-k", label: "Scroll info up", key: tea.KeyMsg{Type: tea.KeyRunes, Alt: true, Runes: []rune{'k'}}},
		)
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
	if m.options.ToggleWatched != nil {
		bindings = append(bindings, helpBinding{keys: "w", label: "Toggle selected item watched", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}}})
	}
	if m.options.ToggleWatchedThrough != nil && m.canWatchThrough() {
		bindings = append(bindings, helpBinding{keys: "W", label: "Toggle watched through selected item", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'W'}}})
	}
	if m.canSwitchEpisode() {
		bindings = append(bindings,
			helpBinding{keys: "n", label: "Load next episode", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'n'}}},
			helpBinding{keys: "p", label: "Load previous episode", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'p'}}},
		)
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
	m.closeAllOverlays()
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
			m.toasts.Err(ToastSort, "Unknown torrent sort key")
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
	case "p":
		m.sortMode = sortPlayedAscending
		m.setContextMode("p")
	case "P":
		m.sortMode = sortPlayedDescending
		m.setContextMode("p")
	case "d", "r":
		m.sortMode = sortRelevance
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		return m, nil
	default:
		m.toasts.Err(ToastSort, "Unknown sort key")
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
			m.toasts.Err(ToastSettings, "Could not save detail mode preference")
		}
	}
}

func (m browserModel[T]) updateQuery(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeAllOverlays()
		m.query = ""
	case "enter":
		query := strings.TrimSpace(m.query)
		if query == "" {
			return m, nil
		}
		m.closeAllOverlays()
		m.loading = true
		m.searching = true
		m.toasts.frame = 0
		m.toasts.Clear()
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

func (m browserModel[T]) updateFilter(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	filter := &m.current().filter
	if m.focusRight {
		filter = &m.right.filter
	}
	switch msg.String() {
	case "enter":
		m.closeAllOverlays()
	case "esc":
		*filter = ""
		m.closeAllOverlays()
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

func inputModal(title, value string, width int, bindings []key.Binding) string {
	input := ansi.Truncate(value, max(1, width-3), "…") + "_"
	return titledModal(title, strings.Join([]string{
		input,
		"",
		renderHelpLine(newHelpModel(), width-2, bindings, helpLineOptions{}),
	}, "\n"), activeBorder.Width(width).Padding(0, 1))
}

func activityModal(frame int, label string) string {
	return activeBorder.Padding(0, 2).Render(spinnerFrames[frame%len(spinnerFrames)] + " " + label)
}

func (m browserModel[T]) helpModal() string {
	bindings := m.filteredHelpBindings()
	title := "Keybindings"
	lines := []string{"Search: " + m.helpFilter + "_", ""}
	if len(bindings) == 0 {
		lines = append(lines, "No matching commands")
	} else {
		selected := clamp(m.helpIndex, len(bindings))
		height := m.height
		if height <= 0 {
			height = 24
		}
		visible := max(1, height-6)
		start := max(0, min(selected-visible/2, len(bindings)-visible))
		end := min(len(bindings), start+visible)
		title += fmt.Sprintf(" · %d-%d/%d", start+1, end, len(bindings))
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
	lines = append(lines, "", renderHelpLine(newHelpModel(), 50, []key.Binding{
		hintBinding("type", "filter"),
		hintBinding("↑/↓", "select"),
		hintBinding("enter", "run"),
		hintBinding("esc", "close"),
	}, helpLineOptions{}))
	return titledModal(title, strings.Join(lines, "\n"), activeBorder.Padding(0, 1))
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
	labels = append(labels, "Two-pane sizes", "Three-pane sizes")
	values = append(values, formatPaneSizes(paneSizeWeights(2, m.paneSizes)), formatPaneSizes(paneSizeWeights(3, m.paneSizes)))
	lines := make([]string, 0, len(labels)+2)
	for i := range labels {
		line := fmt.Sprintf("%-20s  < %-16s >", labels[i], values[i])
		if i == m.settingsIndex {
			line = selectedStyle.Render("> " + line)
		} else {
			line = "  " + line
		}
		lines = append(lines, line)
	}
	lines = append(lines, "", renderHelpLine(newHelpModel(), 54, []key.Binding{
		hintBinding("↑/↓", "item"),
		hintBinding("←/→", "change"),
		hintBinding("enter", "edit"),
		hintBinding("esc", "close"),
	}, helpLineOptions{}))
	return titledModal("Settings", strings.Join(lines, "\n"), activeBorder.Padding(0, 1))
}
