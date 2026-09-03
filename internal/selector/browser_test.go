package selector

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type testChoice struct {
	label       string
	group       string
	terminal    bool
	cached      bool
	quality     int
	year        int
	modes       []ContextMode
	unavailable bool
	cacheKey    string
	direct      bool
	playable    bool
}

func (c testChoice) ContextModes() []ContextMode { return c.modes }
func (c testChoice) Unavailable() bool           { return c.unavailable }
func (c testChoice) CacheKey() string            { return c.cacheKey }

func (c testChoice) Label() string  { return c.label }
func (c testChoice) Group() string  { return c.group }
func (c testChoice) Terminal() bool { return c.terminal }
func (c testChoice) StreamInfo() (StreamInfo, bool) {
	return StreamInfo{Cached: c.cached, CacheApplicable: c.terminal && !c.direct, Playable: c.cached || c.playable, Quality: c.quality}, c.terminal
}
func (c testChoice) SortFields() (string, int, bool) {
	return c.label, c.year, !c.terminal
}

func newBrowser(items ...testChoice) browserModel[testChoice] {
	return browserModel[testChoice]{ctx: context.Background(), levels: []pane[testChoice]{{title: "Search", items: items}}, cachedOnly: true}
}

func runAsync(command tea.Cmd) tea.Msg {
	message := command()
	if batch, ok := message.(tea.BatchMsg); ok {
		return batch[0]()
	}
	return message
}

func TestHistoryRootOmitsSearchAndTabControls(t *testing.T) {
	m := newBrowser(testChoice{label: "movie"})
	m.levels[0].title = "History"
	m.activeQuery = "History"
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "History") {
		t.Fatalf("missing history title: %q", view)
	}
	if strings.Contains(view, "movie/series") || strings.Contains(view, "ctrl-p search") {
		t.Fatalf("history exposed unavailable controls: %q", view)
	}
	for _, binding := range m.filteredHelpBindings() {
		if binding.keys == "Tab" || binding.keys == "Ctrl-P" {
			t.Fatalf("history exposed unavailable binding: %#v", binding)
		}
	}
}

func TestRootTabsUseSelectionIndicators(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", group: "movie"}, testChoice{label: "Dark", group: "series"})
	m.options.ParentGroups = []string{"movie", "series"}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "● Movies") || !strings.Contains(view, "○ Series") {
		t.Fatalf("tabs = %q", view)
	}
}

func TestModePopupSelectsContextualRightColumn(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", modes: []ContextMode{{Key: "y", Name: "Year", Value: "2021"}, {Key: "i", Name: "ID", Value: "tt1160419"}}})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'m'}})
	m = next.(browserModel[testChoice])
	if !m.modeMenu || !strings.Contains(ansi.Strip(m.View()), "[i] ID") {
		t.Fatalf("mode popup missing: %q", ansi.Strip(m.View()))
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	view := ansi.Strip(m.View())
	if m.modeMenu || !strings.Contains(view, "Dune") || !strings.Contains(view, "tt1160419") {
		t.Fatalf("ID mode not rendered: %q", view)
	}
}

func TestModeSelectionPersistsPreference(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", modes: []ContextMode{{Group: "media", Key: "y", Name: "Year", Value: "2021"}, {Group: "media", Key: "i", Name: "ID", Value: "tt1160419"}}})
	var group, key string
	m.options.SaveMode = func(savedGroup, savedKey string) error { group, key = savedGroup, savedKey; return nil }
	m.modeMenu = true
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	if group != "media" || key != "i" || m.mode["media"] != "i" {
		t.Fatalf("saved mode = %q/%q, state = %#v", group, key, m.mode)
	}
}

func TestInvalidPreferredModeFallsBackToDefault(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", modes: []ContextMode{{Group: "media", Key: "y", Name: "Year", Value: "2021"}}})
	m.mode = map[string]string{"media": "removed"}
	if view := ansi.Strip(m.View()); !strings.Contains(view, "2021") {
		t.Fatalf("default mode missing: %q", view)
	}
}

func TestValidEmptyModeDoesNotFallBackToDefault(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", modes: []ContextMode{{Group: "media", Key: "y", Name: "Year", Value: "2021"}, {Group: "media", Key: "r", Name: "Rating", Value: ""}}})
	m.mode = map[string]string{"media": "r"}
	view := ansi.Strip(m.View())
	if strings.Contains(view, "2021") {
		t.Fatalf("empty rating fell back to year: %q", view)
	}
}

func TestUnavailableItemUsesMutedStyleWithoutStrikethrough(t *testing.T) {
	item := testChoice{label: "Future episode", unavailable: true}
	if !item.Unavailable() || unavailableStyle.GetStrikethrough() {
		t.Fatal("unavailable item styling is incorrect")
	}
}

func TestSwitchesBetweenHistoryAndSearchRoots(t *testing.T) {
	m := newBrowser(testChoice{label: "search result", group: "movie"})
	m.options.ParentGroups = []string{"movie", "series"}
	m.options.SearchGroups = []string{"movie", "series"}
	m.options.History = func(context.Context) ([]testChoice, error) {
		return []testChoice{{label: "history item"}}, nil
	}
	m.options.Requery = func(context.Context, string) ([]testChoice, error) { return nil, nil }

	next, command := m.Update(tea.KeyMsg{Type: tea.KeyCtrlH})
	m = next.(browserModel[testChoice])
	if command == nil || !m.loading {
		t.Fatal("ctrl-h did not load history")
	}
	next, _ = m.Update(runAsync(command))
	m = next.(browserModel[testChoice])
	if m.levels[0].title != "History" || len(m.options.ParentGroups) != 0 || m.levels[0].items[0].label != "history item" {
		t.Fatalf("history root = %#v", m)
	}

	next, _ = m.Update(requeryFinished[testChoice]{items: []testChoice{{label: "new result", group: "movie"}}, query: "Dune"})
	m = next.(browserModel[testChoice])
	if m.levels[0].title != "Search results" || len(m.options.ParentGroups) != 2 || m.activeQuery != "Dune" {
		t.Fatalf("search root = %#v", m)
	}
}

func TestBreadcrumbIncludesMediaGroupAndSetsTerminalTitle(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune", group: "movie"})
	m.options.ParentGroups = []string{"movie", "series"}
	m.activeQuery = "Dune"
	m.crumbs = []string{"Season 1"}
	if got := m.breadcrumb(); got != "Movie / Dune / Season 1" {
		t.Fatalf("breadcrumb = %q", got)
	}
	if view := m.View(); !strings.HasPrefix(view, "\x1b]0;Movie / Dune / Season 1\x07") {
		t.Fatalf("terminal title missing: %q", view)
	}
}

func TestCtrlDAndCtrlUMoveHalfPage(t *testing.T) {
	items := make([]testChoice, 30)
	for i := range items {
		items[i].label = fmt.Sprintf("item %d", i)
	}
	m := newBrowser(items...)
	m.height = 24
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = next.(browserModel[testChoice])
	down := m.current().index
	if down <= 0 {
		t.Fatalf("ctrl-d index = %d", down)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlU})
	m = next.(browserModel[testChoice])
	if m.current().index != 0 {
		t.Fatalf("ctrl-u index = %d", m.current().index)
	}
}

func TestGGAndGMoveToListBounds(t *testing.T) {
	m := newBrowser(testChoice{label: "one"}, testChoice{label: "two"}, testChoice{label: "three"})
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'G'}})
	m = next.(browserModel[testChoice])
	if m.current().index != 2 {
		t.Fatalf("G index = %d", m.current().index)
	}
	for range 2 {
		next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		m = next.(browserModel[testChoice])
	}
	if m.current().index != 0 {
		t.Fatalf("gg index = %d", m.current().index)
	}
}

func TestEscapeClearsFilterBeforeNavigatingBack(t *testing.T) {
	m := newBrowser(testChoice{label: "one"})
	m.current().filter = "one"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = next.(browserModel[testChoice])
	if m.current().filter != "" || len(m.levels) != 1 {
		t.Fatalf("escape did not clear filter in place: %#v", m)
	}
}

func TestPaneLayoutUsesSlidingResponsiveWindow(t *testing.T) {
	panes := []visiblePane[testChoice]{{title: "Media"}, {title: "Seasons"}, {title: "Episodes", active: true}, {title: "Torrents"}}

	visible, widths := paneLayout(120, panes)
	if got := paneTitles(visible); got != "Seasons,Episodes,Torrents" || paneWidth(widths) != 120 {
		t.Fatalf("three-pane layout = %q %#v", got, widths)
	}

	visible, widths = paneLayout(80, panes)
	if got := paneTitles(visible); got != "Episodes,Torrents" || paneWidth(widths) != 80 || widths[1] <= widths[0] {
		t.Fatalf("two-pane layout = %q %#v", got, widths)
	}

	visible, widths = paneLayout(60, panes)
	if got := paneTitles(visible); got != "Episodes" || paneWidth(widths) != 60 {
		t.Fatalf("one-pane layout = %q %#v", got, widths)
	}
}

func TestEpisodeChildrenUseCacheUntilRefresh(t *testing.T) {
	m := newBrowser(testChoice{label: "episode", cacheKey: "torrents:episode"})
	loads := 0
	m.load = func(context.Context, testChoice) ([]testChoice, error) {
		loads++
		return []testChoice{{label: fmt.Sprintf("torrent %d", loads), terminal: true}}, nil
	}

	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	m.focusRight = false
	next, command = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if command != nil || loads != 1 || m.right.items[0].label != "torrent 1" {
		t.Fatalf("cache miss: loads=%d command=%v right=%#v", loads, command, m.right.items)
	}

	next, command = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'r'}})
	m = next.(browserModel[testChoice])
	if command == nil {
		t.Fatal("refresh did not reload")
	}
	next, _ = m.Update(command())
	m = next.(browserModel[testChoice])
	if loads != 2 || m.right.items[0].label != "torrent 2" {
		t.Fatalf("refresh result: loads=%d right=%#v", loads, m.right.items)
	}
}

func paneTitles(panes []visiblePane[testChoice]) string {
	titles := make([]string, len(panes))
	for i, pane := range panes {
		titles[i] = pane.title
	}
	return strings.Join(titles, ",")
}

func paneWidth(widths []int) int {
	total := 0
	for _, width := range widths {
		total += width + 2
	}
	return total
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

func TestBrowserKeepsDirectStreamsWhenCachedOnly(t *testing.T) {
	m := newBrowser(testChoice{label: "parent"})
	m.focusRight = true
	m.right.items = []testChoice{{label: "direct", terminal: true, direct: true, playable: true, quality: 1080}}
	if got := m.filteredRight(); len(got) != 1 {
		t.Fatalf("direct streams = %#v", got)
	}
	played := false
	m.options.Play = func(context.Context, testChoice) error { played = true; return nil }
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if command == nil {
		t.Fatal("direct playback did not start")
	}
	_, _ = m.Update(command())
	if !played {
		t.Fatal("direct stream was not played")
	}
}

func TestProviderSettingClearsLoadedStreams(t *testing.T) {
	m := newBrowser(testChoice{label: "episode"})
	m.provider = "torbox"
	m.options.Providers = []string{"torbox", "webstreamr"}
	m.right = pane[testChoice]{items: []testChoice{{label: "cached", terminal: true, cached: true}}}
	m.focusRight = true
	m.loadCache = map[string][]testChoice{"torbox:streams:episode": m.right.items}
	m.settingsIndex = 3
	m.changeSetting(1)
	if m.provider != "webstreamr" || len(m.right.items) != 0 || m.loadCache != nil || m.focusRight {
		t.Fatalf("provider switch state = %#v", m)
	}
}

func TestStaleProviderLoadIsIgnored(t *testing.T) {
	m := newBrowser(testChoice{label: "episode"})
	m.provider = "webstreamr"
	m.loading = true
	next, _ := m.Update(loaded[testChoice]{items: []testChoice{{label: "TorBox stream"}}, provider: "torbox"})
	m = next.(browserModel[testChoice])
	if len(m.right.items) != 0 {
		t.Fatalf("stale results = %#v", m.right.items)
	}
}

func TestStaleLoadGenerationIsIgnored(t *testing.T) {
	m := newBrowser(testChoice{label: "episode"})
	m.provider = "webstreamr"
	m.loadID = 2
	next, _ := m.Update(loaded[testChoice]{items: []testChoice{{label: "old stream"}}, provider: "webstreamr", loadID: 1})
	m = next.(browserModel[testChoice])
	if len(m.right.items) != 0 {
		t.Fatalf("stale results = %#v", m.right.items)
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
	view := ansi.Strip(m.View())
	if !m.searching || !strings.Contains(view, "⠋ Searching") || strings.Contains(view, "Loading...") {
		t.Fatalf("requery loading view = %q", view)
	}
	next, _ = m.Update(runAsync(command))
	m = next.(browserModel[testChoice])
	if m.loading || len(m.levels) != 1 || m.levels[0].items[0].label != "Silo" || m.focusRight || m.activeQuery != "Silo" {
		t.Fatalf("requery result = %#v", m)
	}
}

func TestBrowserTogglesAndRemovesHistory(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.activeQuery = "Search"
	historyItems := []testChoice{{label: "Arrival"}}
	m.options.History = func(context.Context) ([]testChoice, error) {
		return historyItems, nil
	}
	m.options.ToggleHistory = func(_ context.Context, selected testChoice) (bool, error) {
		return selected.label == "Dune", nil
	}
	m.options.RemoveHistory = func(_ context.Context, selected testChoice) error {
		if selected.label != "Arrival" {
			t.Fatalf("removed %q", selected.label)
		}
		historyItems = nil
		return nil
	}
	next, command := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'w'}})
	m = next.(browserModel[testChoice])
	if !m.historyBusy || command == nil {
		t.Fatal("history toggle did not start")
	}
	next, _ = m.Update(runAsync(command))
	m = next.(browserModel[testChoice])
	if m.historyBusy || m.notice != "Added to history" {
		t.Fatalf("history toggle = %#v", m)
	}
	m.levels[0] = pane[testChoice]{title: "History", items: []testChoice{{label: "Arrival"}}}
	m.activeQuery = "History"
	next, command = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'d'}})
	m = next.(browserModel[testChoice])
	if !m.historyBusy || command == nil {
		t.Fatal("history remove did not start")
	}
	next, _ = m.Update(runAsync(command))
	m = next.(browserModel[testChoice])
	if m.historyBusy || len(m.current().items) != 0 || m.notice != "Removed from history" {
		t.Fatalf("history removal = %#v", m)
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

func TestHelpPaletteOpensSettings(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.helpMenu = true
	m.helpFilter = "saved defaults"
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	if m.helpMenu || !m.settingsMenu || !strings.Contains(ansi.Strip(m.View()), "Settings") {
		t.Fatalf("settings did not open: %#v", m)
	}
}

func TestSemicolonTogglesSettings(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	for _, open := range []bool{true, false} {
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{';'}})
		m = next.(browserModel[testChoice])
		if m.settingsMenu != open {
			t.Fatalf("settings open = %t, want %t", m.settingsMenu, open)
		}
	}
}

func TestSettingsCyclesAndPersistsDefaults(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.options.ParentGroups = []string{"movie", "series"}
	m.options.ModeOptions = map[string][]ContextMode{"media": {{Key: "y", Name: "Year"}, {Key: "i", Name: "ID"}}}
	m.mode = map[string]string{"media": "y"}
	var group, mode string
	var quality int
	var cached bool
	m.options.SaveGroup = func(value string) error { group = value; return nil }
	m.options.SaveQuality = func(value int) error { quality = value; return nil }
	m.options.SaveCached = func(value bool) error { cached = value; return nil }
	m.options.SaveMode = func(savedGroup, value string) error { mode = value; group = savedGroup; return nil }
	m.settingsMenu = true

	for _, index := range []int{0, 1, 2, 5} {
		m.settingsIndex = index
		next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRight})
		m = next.(browserModel[testChoice])
	}
	if m.groupIndex != 1 || quality != 2160 || cached || group != "media" || mode != "i" {
		t.Fatalf("settings state: groupIndex=%d quality=%d cached=%t mode=%s/%s", m.groupIndex, quality, cached, group, mode)
	}
}

func TestSettingsAcceptsCustomPlayer(t *testing.T) {
	m := newBrowser(testChoice{label: "Dune"})
	m.settingsMenu = true
	m.settingsIndex = 4
	var saved string
	m.options.SavePlayer = func(value string) error { saved = value; return nil }
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(browserModel[testChoice])
	for _, message := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("my-player")}, {Type: tea.KeyEnter}} {
		next, _ = m.Update(message)
		m = next.(browserModel[testChoice])
	}
	if m.customPlayer || m.player != "my-player" || saved != "my-player" {
		t.Fatalf("custom player = %q, saved = %q", m.player, saved)
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
