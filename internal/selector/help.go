package selector

import (
	"strings"

	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

type browserKeyMap struct {
	Keys           key.Binding
	Groups         key.Binding
	Mode           key.Binding
	Info           key.Binding
	Sort           key.Binding
	Navigate       key.Binding
	Home           key.Binding
	History        key.Binding
	Search         key.Binding
	Filter         key.Binding
	Quit           key.Binding
	Episode        key.Binding
	Cached         key.Binding
	Quality        key.Binding
	Stop           key.Binding
	Watched        key.Binding
	WatchedThrough key.Binding
	Remove         key.Binding
	Back           key.Binding
	Refresh        key.Binding
}

func browserKeys() browserKeyMap {
	return browserKeyMap{
		Keys:           key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "help")),
		Groups:         key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "movie/series")),
		Mode:           key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mode")),
		Info:           key.NewBinding(key.WithKeys("i"), key.WithHelp("i", "info")),
		Sort:           key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		Navigate:       key.NewBinding(key.WithKeys("h", "j", "k", "l"), key.WithHelp("hjkl", "move")),
		Home:           key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "home")),
		History:        key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl-h", "history")),
		Search:         key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl-p", "search")),
		Filter:         key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Quit:           key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Episode:        key.NewBinding(key.WithKeys("n", "p"), key.WithHelp("n/p", "episode")),
		Cached:         key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cached/all")),
		Quality:        key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "quality")),
		Stop:           key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Watched:        key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watched")),
		WatchedThrough: key.NewBinding(key.WithKeys("w", "W"), key.WithHelp("w/W", "watched")),
		Remove:         key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove")),
		Back:           key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		Refresh:        key.NewBinding(key.WithKeys("r", "f5"), key.WithHelp("r/F5", "refresh")),
	}
}

func newHelpModel() help.Model {
	h := help.New()
	h.ShortSeparator = " "
	h.Styles = help.Styles{
		ShortKey:       headerStyle,
		ShortDesc:      hintStyle,
		ShortSeparator: hintStyle,
		Ellipsis:       hintStyle,
	}
	return h
}

func hintBinding(keys, description string) key.Binding {
	return key.NewBinding(key.WithKeys(keys), key.WithHelp(keys, description))
}

type helpLineOptions struct {
	Right       string
	RightColumn bool
}

func renderHelpLine(model help.Model, width int, bindings []key.Binding, options helpLineOptions) string {
	if !options.RightColumn {
		model.Width = width
		return ansi.Truncate(model.ShortHelpView(bindings), width, "…")
	}
	right := versionStyle.Render(plainLabel(options.Right))
	rightWidth := lipgloss.Width(right)
	if rightWidth >= width {
		return right
	}
	leftWidth := width
	if rightWidth > 0 {
		leftWidth = max(1, width-rightWidth-1)
	}
	model.Width = leftWidth
	left := ansi.Truncate(model.ShortHelpView(bindings), leftWidth, "…")
	if rightWidth == 0 {
		return left
	}
	gap := max(1, width-lipgloss.Width(left)-rightWidth)
	return left + strings.Repeat(" ", gap) + right
}

func (m browserModel[T]) shortHelp(k browserKeyMap) []key.Binding {
	rightStreams := m.focusRight && m.rightHasStreams()
	bindings := []key.Binding{k.Navigate, k.Keys, k.Info}
	if m.playback.busy() {
		bindings = append(bindings, k.Stop)
	}
	if m.canRefresh() {
		bindings = append(bindings, k.Refresh)
	}
	if m.options.ToggleWatched != nil && !rightStreams {
		if m.options.ToggleWatchedThrough != nil && m.canWatchThrough() {
			bindings = append(bindings, k.WatchedThrough)
		} else {
			bindings = append(bindings, k.Watched)
		}
	}
	if m.inHistoryRoot() && m.options.RemoveHistory != nil && m.options.History != nil {
		bindings = append(bindings, k.Remove)
	}
	if rightStreams {
		if m.rightCacheApplicable() {
			bindings = append(bindings, k.Cached)
		}
		bindings = append(bindings, k.Quality)
	}
	if m.canSwitchEpisode() {
		bindings = append(bindings, k.Episode)
	}
	if !m.focusRight && len(m.levels) == 1 && len(m.options.ParentGroups) > 1 {
		bindings = append(bindings, k.Groups)
	}
	bindings = append(bindings, k.Filter)
	if m.focusRight || len(m.levels) > 1 || m.right.title != "" {
		return append(bindings, k.Home, k.Back)
	}
	return append(bindings, k.Quit)
}
