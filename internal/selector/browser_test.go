package selector

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type testChoice struct {
	label    string
	group    string
	terminal bool
	cached   bool
	quality  int
	year     int
}

func (c testChoice) Label() string  { return c.label }
func (c testChoice) Group() string  { return c.group }
func (c testChoice) Terminal() bool { return c.terminal }
func (c testChoice) StreamInfo() (bool, int, bool) {
	return c.cached, c.quality, c.terminal
}
func (c testChoice) SortFields() (string, int, bool) {
	return c.label, c.year, !c.terminal
}

func newBrowser(items ...testChoice) browserModel[testChoice] {
	return browserModel[testChoice]{ctx: context.Background(), levels: []pane[testChoice]{{title: "Search", items: items}}, cachedOnly: true}
}

func TestBrowserLoadsAndChoosesTerminal(t *testing.T) {
	m := newBrowser(testChoice{label: "movie"})
	m.load = func(_ context.Context, parent testChoice) ([]testChoice, error) {
		return []testChoice{{label: "torrent", terminal: true, cached: true}}, nil
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !m.loading || command == nil {
		t.Fatal("enter did not start loading")
	}
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	if !m.focusRight || len(m.right.items) != 1 {
		t.Fatalf("right pane not focused: %#v", m)
	}
	next, command = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !m.chosen || m.choice.label != "torrent" || command == nil {
		t.Fatalf("terminal not chosen: %#v", m)
	}
}

func TestRightOpensSelectedLeftItem(t *testing.T) {
	m := newBrowser(testChoice{label: "movie"})
	m.right.items = []testChoice{{label: "stale stream"}}
	m.load = func(_ context.Context, _ testChoice) ([]testChoice, error) {
		return []testChoice{{label: "stream"}}, nil
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(browserModel[testChoice])
	if !m.loading || command == nil {
		t.Fatalf("right did not confirm left item: %#v", m)
	}
}

func TestRightConfirmsSelectedRightItem(t *testing.T) {
	m := newBrowser(testChoice{label: "series"})
	m.focusRight = true
	m.crumbs = []string{"series"}
	m.right = pane[testChoice]{title: "Seasons", items: []testChoice{{label: "season 1"}}}
	m.load = func(_ context.Context, selected testChoice) ([]testChoice, error) {
		if selected.label != "season 1" {
			t.Fatalf("selected = %q", selected.label)
		}
		return []testChoice{{label: "episode 1"}}, nil
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = next.(browserModel[testChoice])
	if len(m.levels) != 2 || !m.loading || command == nil {
		t.Fatalf("right did not confirm right item: %#v", m)
	}
}

func TestBackAtRootDoesNotExit(t *testing.T) {
	for _, key := range []tea.KeyMsg{{Type: tea.KeyLeft}, {Type: tea.KeyRunes, Runes: []rune{'h'}}, {Type: tea.KeyEscape}} {
		m := newBrowser(testChoice{label: "movie"})
		next, command := m.Update(key)
		m = next.(browserModel[testChoice])
		if command != nil || len(m.levels) != 1 {
			t.Fatalf("%q exited or changed root: %#v", key.String(), m)
		}
	}
}

func TestBrowserPromotesChildrenAndBacktracks(t *testing.T) {
	m := newBrowser(testChoice{label: "series"})
	m.load = func(_ context.Context, selected testChoice) ([]testChoice, error) {
		switch selected.label {
		case "series":
			return []testChoice{{label: "season 1"}}, nil
		case "season 1":
			return []testChoice{{label: "episode 1"}}, nil
		default:
			return nil, nil
		}
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	next, command = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if len(m.levels) != 2 || m.levels[1].items[0].label != "season 1" {
		t.Fatalf("children not promoted: %#v", m.levels)
	}
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	if m.right.items[0].label != "episode 1" || len(m.crumbs) != 2 {
		t.Fatalf("episode pane or crumbs wrong: %#v", m)
	}
	m.focusRight = false
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = next.(browserModel[testChoice])
	if len(m.levels) != 1 || !m.focusRight || m.right.items[0].label != "season 1" {
		t.Fatalf("backtracking failed: %#v", m)
	}
}

func TestBrowserFiltersActivePane(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"}, testChoice{label: "Arrival"})
	m.filtering = true
	for _, key := range []rune("arr") {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = next.(browserModel[testChoice])
	}
	items := m.filteredCurrent()
	if len(items) != 1 || items[0].index != 1 || items[0].item.label != "Arrival" {
		t.Fatalf("filtered items = %#v", items)
	}
}

func TestBrowserShowsLoadingErrorAndBreadcrumb(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.crumbs = []string{"Dune", "Season 1"}
	m.loading = true
	m.width, m.height = 80, 16
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Search / Dune / Season 1") || !strings.Contains(view, "Loading...") {
		t.Fatalf("loading view = %q", view)
	}
	m.loading = false
	m.err = errors.New("service unavailable")
	view = ansi.Strip(m.View())
	if !strings.Contains(view, "Error: service unavailable") || !strings.Contains(view, "Press Enter to retry") {
		t.Fatalf("error view = %q", view)
	}
}

func TestBrowserTabsParentGroups(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", group: "movie"}, testChoice{label: "Silo", group: "series"})
	m.options.ParentGroups = []string{"movie", "series"}
	if got := m.filteredCurrent(); len(got) != 1 || got[0].item.label != "Dune" {
		t.Fatalf("movie tab = %#v", got)
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(browserModel[testChoice])
	if got := m.filteredCurrent(); len(got) != 1 || got[0].item.label != "Silo" {
		t.Fatalf("series tab = %#v", got)
	}
}

func TestBrowserPersistsAndRestoresParentGroup(t *testing.T) {
	saved := ""
	m := newBrowser(testChoice{label: "Dune", group: "movie"}, testChoice{label: "Silo", group: "series"})
	m.options.ParentGroups = []string{"movie", "series"}
	m.options.SaveGroup = func(group string) error { saved = group; return nil }
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = next.(browserModel[testChoice])
	if saved != "series" {
		t.Fatalf("saved group = %q", saved)
	}
	if got := preferredGroupIndex(m.options.ParentGroups, saved); got != 1 {
		t.Fatalf("restored group index = %d", got)
	}
	if got := preferredGroupIndex(m.options.ParentGroups, "invalid"); got != 0 {
		t.Fatalf("invalid group index = %d", got)
	}
}

func TestBrowserSortsRootAndRestoresRelevance(t *testing.T) {
	m := newBrowser(
		testChoice{label: "Zulu", group: "movie", year: 2000},
		testChoice{label: "Alpha", group: "movie", year: 2020},
		testChoice{label: "Unknown", group: "movie"},
	)
	m.options.ParentGroups = []string{"movie", "series"}

	setSort := func(key rune) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m = next.(browserModel[testChoice])
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = next.(browserModel[testChoice])
	}
	labels := func() string {
		items := m.filteredCurrent()
		values := make([]string, len(items))
		for i, item := range items {
			values[i] = item.item.label
		}
		return strings.Join(values, ",")
	}

	setSort('a')
	if got := labels(); got != "Alpha,Unknown,Zulu" {
		t.Fatalf("name ascending = %q", got)
	}
	setSort('A')
	if got := labels(); got != "Zulu,Unknown,Alpha" {
		t.Fatalf("name descending = %q", got)
	}
	setSort('y')
	if got := labels(); got != "Zulu,Alpha,Unknown" {
		t.Fatalf("year ascending = %q", got)
	}
	setSort('Y')
	if got := labels(); got != "Alpha,Zulu,Unknown" {
		t.Fatalf("year descending = %q", got)
	}
	setSort('r')
	if got := labels(); got != "Zulu,Alpha,Unknown" {
		t.Fatalf("relevance = %q", got)
	}
	setSort('a')
	setSort('d')
	if got := labels(); got != "Zulu,Alpha,Unknown" {
		t.Fatalf("default alias = %q", got)
	}
}

func TestBrowserShowsAndCancelsSortMenu(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", group: "movie"})
	m.options.ParentGroups = []string{"movie", "series"}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
	m = next.(browserModel[testChoice])
	if !m.sortMenu || !strings.Contains(ansi.Strip(m.View()), "a   Name ascending") {
		t.Fatalf("sort menu not visible: %q", ansi.Strip(m.View()))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(browserModel[testChoice])
	if m.sortMenu || m.sortMode != sortRelevance {
		t.Fatalf("sort menu not cancelled: %#v", m)
	}
}

func TestOverlayComposesModalOverBase(t *testing.T) {
	base := "top line\nleft pane content and right pane content\nbottom line\n"
	modal := "modal\nmenu"
	view := overlay(base, modal, 45)
	if !strings.Contains(view, "top line") || !strings.Contains(view, "modal") || !strings.Contains(view, "menu") {
		t.Fatalf("overlay = %q", view)
	}
	if strings.Count(view, "\n") != strings.Count(base, "\n") {
		t.Fatalf("overlay changed layer height: %q", view)
	}
}

func TestToastRendersAndExpiresByGeneration(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", group: "movie"})
	m.options.ParentGroups = []string{"movie", "series"}
	m.sortMenu = true
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'z'}})
	m = next.(browserModel[testChoice])
	if m.notice != "Unknown sort key" || m.toastID == 0 || command == nil {
		t.Fatalf("toast not scheduled: %#v", m)
	}
	if !strings.Contains(ansi.Strip(m.View()), "Unknown sort key") {
		t.Fatalf("toast not rendered: %q", ansi.Strip(m.View()))
	}
	next, _ = m.Update(toastExpired{id: m.toastID - 1})
	m = next.(browserModel[testChoice])
	if m.notice == "" {
		t.Fatal("stale timer cleared current toast")
	}
	next, _ = m.Update(toastExpired{id: m.toastID})
	m = next.(browserModel[testChoice])
	if m.notice != "" {
		t.Fatalf("toast did not expire: %q", m.notice)
	}
}

func TestToastOverlaysBottomRight(t *testing.T) {
	base := "top\nsecond line\nthird line\nfourth line\nbottom\n"
	view := ansi.Strip(toastOverlay(base, "network error", 50))
	lines := strings.Split(strings.TrimSuffix(view, "\n"), "\n")
	if !strings.Contains(strings.Join(lines[len(lines)-4:], "\n"), "network error") {
		t.Fatalf("toast not near bottom: %q", view)
	}
	if !strings.HasPrefix(lines[0], "top") {
		t.Fatalf("toast damaged base: %q", view)
	}
}

func TestLoadErrorCreatesToast(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	next, command := m.Update(loaded[testChoice]{err: errors.New("network error")})
	m = next.(browserModel[testChoice])
	if m.notice != "Load failed: network error" || command == nil {
		t.Fatalf("load error toast = %#v", m)
	}
}

func TestBrowserFiltersCacheAndQuality(t *testing.T) {
	m := newBrowser(testChoice{label: "parent"})
	m.right.items = []testChoice{{label: "1080p", terminal: true, cached: true, quality: 1080}, {label: "2160p", terminal: true, quality: 2160}}
	m.quality = 1080
	if got := m.filteredRight(); len(got) != 1 || got[0].item.quality != 1080 {
		t.Fatalf("cached quality = %#v", got)
	}
	m.cachedOnly = false
	m.quality = 2160
	if got := m.filteredRight(); len(got) != 1 || got[0].item.cached {
		t.Fatalf("all quality = %#v", got)
	}
}

func TestBrowserSortsTorrentResults(t *testing.T) {
	m := newBrowser(testChoice{label: "parent"})
	m.focusRight = true
	m.cachedOnly = false
	m.right.items = []testChoice{
		{label: "Zulu", terminal: true, cached: true, quality: 1080},
		{label: "Alpha", terminal: true, cached: false, quality: 2160},
		{label: "Beta", terminal: true, cached: true, quality: 720},
	}
	labels := func() string {
		items := m.filteredRight()
		values := make([]string, len(items))
		for i, item := range items {
			values[i] = item.item.label
		}
		return strings.Join(values, ",")
	}
	setSort := func(key rune) {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'s'}})
		m = next.(browserModel[testChoice])
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = next.(browserModel[testChoice])
	}
	setSort('Q')
	if got := labels(); got != "Alpha,Zulu,Beta" {
		t.Fatalf("quality descending = %q", got)
	}
	setSort('c')
	if got := labels(); got != "Zulu,Beta,Alpha" {
		t.Fatalf("cached first = %q", got)
	}
	setSort('N')
	if got := labels(); got != "Zulu,Beta,Alpha" {
		t.Fatalf("name descending = %q", got)
	}
	setSort('d')
	if got := labels(); got != "Zulu,Alpha,Beta" {
		t.Fatalf("default ranking = %q", got)
	}
	setSort('q')
	setSort('r')
	if got := labels(); got != "Zulu,Alpha,Beta" {
		t.Fatalf("default ranking alias = %q", got)
	}
}

func TestFilterEditingShortcuts(t *testing.T) {
	m := newBrowser(testChoice{label: "one two three"})
	m.filtering = true
	m.current().filter = "one two three"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = next.(browserModel[testChoice])
	if m.current().filter != "one two" {
		t.Fatalf("ctrl-w filter = %q", m.current().filter)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = next.(browserModel[testChoice])
	if m.current().filter != "" {
		t.Fatalf("ctrl-u filter = %q", m.current().filter)
	}
}

func TestFilterAcceptsSpaces(t *testing.T) {
	m := newBrowser(testChoice{label: "one piece"})
	m.filtering = true
	for _, message := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("one")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("piece")},
	} {
		next, _ := m.Update(message)
		m = next.(browserModel[testChoice])
	}
	if m.current().filter != "one piece" || len(m.filteredCurrent()) != 1 {
		t.Fatalf("filter = %q, matches = %#v", m.current().filter, m.filteredCurrent())
	}
}

func TestFilterAndSearchRenderAsModalLayers(t *testing.T) {
	m := newBrowser(testChoice{label: "One Piece"})
	m.activeQuery = "One Piece"
	m.filtering = true
	m.current().filter = "piece"
	filterView := ansi.Strip(m.View())
	if !strings.Contains(filterView, "Filter active pane") || !strings.Contains(filterView, "One Piece") {
		t.Fatalf("filter modal = %q", filterView)
	}
	m.filtering = false
	m.querying = true
	m.query = "Family Guy"
	queryView := ansi.Strip(m.View())
	if !strings.Contains(queryView, "Search") || !strings.Contains(queryView, "Family Guy_") || !strings.Contains(queryView, "One Piece") {
		t.Fatalf("search modal = %q", queryView)
	}
}

func TestQualityCyclePersists(t *testing.T) {
	saved := -1
	m := newBrowser(testChoice{label: "parent"})
	m.focusRight = true
	m.right.items = []testChoice{{label: "stream", terminal: true, cached: true}}
	m.options.SaveQuality = func(quality int) error { saved = quality; return nil }
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'v'}})
	m = next.(browserModel[testChoice])
	if m.quality != 2160 || saved != 2160 {
		t.Fatalf("quality = %d, saved = %d", m.quality, saved)
	}
}

func TestBrowserPlaybackKeepsSessionOpen(t *testing.T) {
	played := false
	m := newBrowser(testChoice{label: "parent"})
	m.focusRight = true
	m.right.items = []testChoice{{label: "stream", terminal: true, cached: true}}
	m.options.Play = func(context.Context, testChoice) error { played = true; return nil }
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !m.playing || m.chosen || command == nil {
		t.Fatalf("playback did not remain in session: %#v", m)
	}
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	if !played || m.playing || m.notice != "Playback launched" {
		t.Fatalf("playback completion = %#v, played = %t", m, played)
	}
}

func TestBrowserStopsPlayback(t *testing.T) {
	m := newBrowser(testChoice{label: "parent"})
	m.focusRight = true
	m.right.items = []testChoice{{label: "stream", terminal: true, cached: true}}
	m.options.Play = func(ctx context.Context, _ testChoice) error { <-ctx.Done(); return ctx.Err() }
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(browserModel[testChoice])
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	if m.playing || m.notice != "Playback stopped" {
		t.Fatalf("stopped playback = %#v", m)
	}
}

func TestBrowserRequeriesRoot(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.options.InitialTitle = "Search results"
	m.activeQuery = "Dune"
	m.options.Requery = func(_ context.Context, query string) ([]testChoice, error) {
		if query != "Silo" {
			t.Fatalf("query = %q", query)
		}
		return []testChoice{{label: "Silo"}}, nil
	}
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = next.(browserModel[testChoice])
	for _, key := range "Silo" {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{key}})
		m = next.(browserModel[testChoice])
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if !m.loading || command == nil {
		t.Fatalf("requery did not start: %#v", m)
	}
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	if m.loading || len(m.levels) != 1 || m.levels[0].items[0].label != "Silo" || m.focusRight || m.activeQuery != "Silo" {
		t.Fatalf("requery result = %#v", m)
	}
}

func TestBrowserQueryAcceptsSpaces(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.querying = true
	for _, message := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("one")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("piece")},
	} {
		next, _ := m.Update(message)
		m = next.(browserModel[testChoice])
	}
	if m.query != "one piece" {
		t.Fatalf("query = %q", m.query)
	}
}

func TestHelpPaletteFiltersAndRunsSelectedAction(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.options.Requery = func(context.Context, string) ([]testChoice, error) { return nil, nil }
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = next.(browserModel[testChoice])
	for _, message := range []tea.KeyMsg{
		{Type: tea.KeyRunes, Runes: []rune("new")},
		{Type: tea.KeySpace},
		{Type: tea.KeyRunes, Runes: []rune("search")},
	} {
		next, _ = m.Update(message)
		m = next.(browserModel[testChoice])
	}
	bindings := m.filteredHelpBindings()
	if len(bindings) != 1 || bindings[0].keys != "Ctrl-P" {
		t.Fatalf("filtered bindings = %#v", bindings)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if m.helpMenu || !m.querying {
		t.Fatalf("selected action did not open search: %#v", m)
	}
}

func TestHelpPaletteCanSelectFilterAction(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.helpMenu = true
	m.helpFilter = "active pane"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if m.helpMenu || !m.filtering {
		t.Fatalf("selected action did not open filter: %#v", m)
	}
}

func TestBrowserBreadcrumbUsesActiveQuery(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.activeQuery = "science fiction"
	view := ansi.Strip(m.View())
	if !strings.HasPrefix(view, "science fiction\n") {
		t.Fatalf("breadcrumb = %q", view)
	}
}
