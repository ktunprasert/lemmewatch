package selector

import (
	"github.com/charmbracelet/bubbles/help"
	"github.com/charmbracelet/bubbles/key"
)

type browserKeyMap struct {
	Keys    key.Binding
	Groups  key.Binding
	Mode    key.Binding
	Sort    key.Binding
	Focus   key.Binding
	Home    key.Binding
	Move    key.Binding
	Open    key.Binding
	OpenSel key.Binding
	History key.Binding
	Search  key.Binding
	Filter  key.Binding
	Quit    key.Binding
	Episode key.Binding
	Cached  key.Binding
	Quality key.Binding
	Stop    key.Binding
	Watched key.Binding
	Remove  key.Binding
	Back    key.Binding
}

func browserKeys() browserKeyMap {
	return browserKeyMap{
		Keys:    key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "keys")),
		Groups:  key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "movie/series")),
		Mode:    key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "mode")),
		Sort:    key.NewBinding(key.WithKeys("s"), key.WithHelp("s", "sort")),
		Focus:   key.NewBinding(key.WithKeys("h", "l"), key.WithHelp("h/l", "focus")),
		Home:    key.NewBinding(key.WithKeys("H"), key.WithHelp("H", "home")),
		Move:    key.NewBinding(key.WithKeys("j", "k"), key.WithHelp("j/k", "move")),
		Open:    key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open")),
		OpenSel: key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open/select")),
		History: key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl-h", "history")),
		Search:  key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl-p", "search")),
		Filter:  key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "filter")),
		Quit:    key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
		Episode: key.NewBinding(key.WithKeys("n", "p"), key.WithHelp("n/p", "episode")),
		Cached:  key.NewBinding(key.WithKeys("c"), key.WithHelp("c", "cached/all")),
		Quality: key.NewBinding(key.WithKeys("v"), key.WithHelp("v", "quality")),
		Stop:    key.NewBinding(key.WithKeys("x"), key.WithHelp("x", "stop")),
		Watched: key.NewBinding(key.WithKeys("w"), key.WithHelp("w", "watched")),
		Remove:  key.NewBinding(key.WithKeys("d"), key.WithHelp("d", "remove")),
		Back:    key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
	}
}

func newHelpModel() help.Model {
	h := help.New()
	h.ShortSeparator = "  "
	h.Styles = help.Styles{
		ShortKey:       headerStyle,
		ShortDesc:      hintStyle,
		ShortSeparator: hintStyle,
		Ellipsis:       hintStyle,
	}
	return h
}

func (m browserModel[T]) shortHelp(k browserKeyMap) []key.Binding {
	rightStreams := m.focusRight && m.rightHasStreams()
	bindings := make([]key.Binding, 0, 16)
	if m.playback.busy() {
		bindings = append(bindings, k.Stop)
	}
	if m.options.ToggleWatched != nil && !rightStreams {
		bindings = append(bindings, k.Watched)
	}
	if m.inHistoryRoot() && m.options.RemoveHistory != nil && m.options.History != nil {
		bindings = append(bindings, k.Remove)
	}
	if m.focusRight {
		if rightStreams && m.rightCacheApplicable() {
			bindings = append(bindings, k.Cached, k.Quality)
		}
		if m.canSwitchEpisode() {
			bindings = append(bindings, k.Episode)
		}
		bindings = append(bindings, k.Focus, k.Home, k.Move, k.OpenSel, k.Filter, k.Back)
		return bindings
	}
	if len(m.options.ParentGroups) > 1 {
		bindings = append(bindings, k.Groups)
	}
	bindings = append(bindings, k.Keys, k.Mode, k.Sort, k.Focus, k.Home, k.Move, k.Open)
	if m.options.Requery != nil {
		bindings = append(bindings, k.History, k.Search)
	}
	return append(bindings, k.Filter, k.Quit)
}
