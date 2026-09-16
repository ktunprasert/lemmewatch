package selector

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"lemmewatch/internal/model"
)

func autoplayBrowser(t *testing.T) browserModel[testChoice] {
	t.Helper()
	season := testChoice{label: "Season 1", cacheKey: "season:show:1"}
	episode1 := testChoice{label: "Episode 1", cacheKey: "streams:1", watchID: "show", watchKeys: []string{"1:1"}}
	episode2 := testChoice{label: "Episode 2", cacheKey: "streams:2", watchID: "show", watchKeys: []string{"1:2"}}
	m := newBrowser(season)
	m.levels = append(m.levels, pane[testChoice]{title: "Episodes", items: []testChoice{episode1, episode2}})
	m.focusRight = true
	m.right = pane[testChoice]{title: "Streams", items: []testChoice{{label: "current", terminal: true, cached: true, watchID: "show", watchKeys: []string{"1:1"}}}}
	m.crumbs = []string{"Season 1", "Episode 1"}
	m.provider = "web"
	m.autoplay = true
	m.quality = 1080
	m.playbackPreferences = model.PlaybackPreferences{AudioLanguages: []string{"ja"}}
	m.load = func(_ context.Context, selected testChoice) ([]testChoice, error) {
		if selected.cacheKey != "streams:2" {
			t.Fatalf("prefetched %q", selected.label)
		}
		return []testChoice{
			{label: "wrong quality", terminal: true, cached: true, quality: 720, watchID: "show", watchKeys: []string{"1:2"}},
			{label: "other language", terminal: true, cached: true, quality: 1080, audioLanguages: []string{"de"}, watchID: "show", watchKeys: []string{"1:2"}},
			{label: "preferred", terminal: true, cached: true, quality: 1080, audioLanguages: []string{"ja"}, watchID: "show", watchKeys: []string{"1:2"}},
		}, nil
	}
	m.options.Play = func(context.Context, testChoice, func(PlaybackStatus)) error { return nil }
	ctx, cancel := context.WithCancel(m.ctx)
	m.playback.start(ctx, nil, make(chan PlaybackStatus), cancel, 0)
	m.autoplayPlayback = autoplayModel[testChoice]{playID: m.playback.id, cursor: m.captureAutoplayCursor()}
	return m
}

func TestAutoplayPrefetchesWithoutSwitchingThenStartsRankedStream(t *testing.T) {
	m := autoplayBrowser(t)
	playID := m.playback.id
	next, command := m.Update(playStatus{id: playID, status: PlaybackStatus{Position: 940, Duration: 1000}})
	m = next.(browserModel[testChoice])
	batch, ok := command().(tea.BatchMsg)
	if !ok || len(batch) != 2 || !m.autoplayPlayback.prefetching {
		t.Fatalf("prefetch command = %#v, state = %#v", command, m.autoplayPlayback)
	}
	if m.current().index != 0 || m.right.items[0].label != "current" {
		t.Fatal("prefetch switched visible episode")
	}

	next, _ = m.Update(batch[1]())
	m = next.(browserModel[testChoice])
	if m.autoplayPlayback.next == nil || m.current().index != 0 || m.right.items[0].label != "current" {
		t.Fatalf("prefetch changed navigation: %#v", m.autoplayPlayback.next)
	}

	next, command = m.Update(playFinished{id: playID, status: PlaybackStatus{Position: 1000, Duration: 1000, Completed: true}})
	m = next.(browserModel[testChoice])
	if command == nil || !m.playback.running || m.playback.id == playID || m.current().index != 1 {
		t.Fatalf("next playback did not start: %#v", m)
	}
	visible := m.filteredRight()
	if len(visible) != 2 || visible[m.right.index].item.label != "preferred" {
		t.Fatalf("autoplay stream = %#v, index = %d", visible, m.right.index)
	}
	if !m.options.Watched["show:1:2"] {
		t.Fatal("autoplay did not update watched state")
	}
	m.playback.finish()
}

func TestAutoplayRequiresLatestPlaybackPositionToBeComplete(t *testing.T) {
	m := autoplayBrowser(t)
	playID := m.playback.id
	next, _ := m.Update(playStatus{id: playID, status: PlaybackStatus{Position: 990, Duration: 1000, Completed: true}})
	m = next.(browserModel[testChoice])
	next, _ = m.Update(playStatus{id: playID, status: PlaybackStatus{Position: 500, Duration: 1000}})
	m = next.(browserModel[testChoice])
	m.autoplayPlayback.next = &episodeSwitched[testChoice]{
		episodes: m.current().items, streams: []testChoice{{label: "next", terminal: true, cached: true, quality: 1080}},
		seasonIndex: 0, episodeIndex: 1, seasonLabel: "Season 1", episodeLabel: "Episode 2", found: true, provider: m.provider,
	}
	next, _ = m.Update(playFinished{id: playID, status: PlaybackStatus{Position: 500, Duration: 1000}})
	m = next.(browserModel[testChoice])
	if m.playback.busy() || m.current().index != 0 {
		t.Fatalf("incomplete playback autoplayed: %#v", m)
	}
}

func TestAutoplayAdvancesAtCompletionThreshold(t *testing.T) {
	m := autoplayBrowser(t)
	playID := m.playback.id
	oldContext := m.playback.ctx
	m.autoplayPlayback.next = &episodeSwitched[testChoice]{
		episodes: m.current().items, streams: []testChoice{{label: "next", terminal: true, cached: true, quality: 1080}},
		seasonIndex: 0, episodeIndex: 1, seasonLabel: "Season 1", episodeLabel: "Episode 2", found: true, provider: m.provider,
	}

	next, _ := m.Update(playStatus{id: playID, status: PlaybackStatus{Position: 985, Duration: 1000, Completed: true}})
	m = next.(browserModel[testChoice])
	select {
	case <-oldContext.Done():
	default:
		t.Fatal("completion threshold did not stop current playback")
	}
	if !m.autoplayPlayback.advancing {
		t.Fatal("autoplay advance was not marked")
	}

	next, command := m.Update(playFinished{id: playID, err: context.Canceled, status: PlaybackStatus{Position: 985, Duration: 1000, Completed: true}})
	m = next.(browserModel[testChoice])
	if command == nil || !m.playback.running || m.playback.id == playID || m.current().index != 1 {
		t.Fatalf("threshold advance did not start next playback: %#v", m)
	}
	m.playback.finish()
}

func TestManualStopCancelsPendingAutoplayAdvance(t *testing.T) {
	m := autoplayBrowser(t)
	playID := m.playback.id
	m.autoplayPlayback.completed = true
	m.autoplayPlayback.next = &episodeSwitched[testChoice]{
		episodes: m.current().items, streams: []testChoice{{label: "next", terminal: true, cached: true, quality: 1080}},
		seasonIndex: 0, episodeIndex: 1, found: true, provider: m.provider,
	}
	if !m.requestAutoplayAdvance() {
		t.Fatal("advance did not begin")
	}

	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = next.(browserModel[testChoice])
	next, _ = m.Update(playFinished{id: playID, err: context.Canceled, status: PlaybackStatus{Position: 985, Duration: 1000, Completed: true}})
	m = next.(browserModel[testChoice])
	if m.playback.busy() || m.current().index != 0 || m.playback.id != playID {
		t.Fatalf("manual stop autoplayed: %#v", m)
	}
}

func TestAutoplayPrefetchCrossesSeasonBoundaryAndStopsAtFutureEpisode(t *testing.T) {
	season1 := testChoice{label: "Season 1", cacheKey: "season:show:1"}
	season2 := testChoice{label: "Season 2", cacheKey: "season:show:2"}
	episode1 := testChoice{label: "Episode 1", cacheKey: "streams:1"}
	future := testChoice{label: "Episode 1", cacheKey: "streams:2", unavailable: true}
	m := newBrowser(season1, season2)
	m.levels = append(m.levels, pane[testChoice]{title: "Episodes", items: []testChoice{episode1}})
	m.focusRight = true
	m.right.items = []testChoice{{label: "current", terminal: true, cached: true}}
	m.provider, m.autoplay = "web", true
	m.load = func(_ context.Context, selected testChoice) ([]testChoice, error) {
		if selected.cacheKey == season2.cacheKey {
			return []testChoice{future}, nil
		}
		t.Fatal("future episode streams must not load")
		return nil, nil
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.playback.start(ctx, nil, make(chan PlaybackStatus), cancel, 0)
	m.autoplayPlayback = autoplayModel[testChoice]{playID: m.playback.id, cursor: m.captureAutoplayCursor()}
	message := m.startAutoplayPrefetch()().(autoplayPrefetched[testChoice])
	if message.next.found || message.next.err != nil {
		t.Fatalf("future boundary = %#v", message.next)
	}
	m.playback.finish()
}

func TestAutoplayDoesNotMoveUnrelatedBrowserPane(t *testing.T) {
	m := autoplayBrowser(t)
	m.levels[0].items = []testChoice{{label: "Other season", cacheKey: "season:other:1"}}
	m.right = pane[testChoice]{items: []testChoice{{label: "other one"}, {label: "other two"}}, index: 1}
	m.autoplayPlayback.next = &episodeSwitched[testChoice]{
		episodes:    []testChoice{{label: "Episode 1"}, {label: "Episode 2"}},
		streams:     []testChoice{{label: "next", terminal: true, cached: true, quality: 1080}},
		seasonIndex: 0, episodeIndex: 1, found: true, provider: m.provider,
	}

	next, command := m.startPrefetchedPlayback()
	m = next.(browserModel[testChoice])
	if command == nil || len(m.right.items) != 2 || m.right.items[0].label != "other one" || m.right.index != 1 {
		t.Fatalf("unrelated pane changed: %#v", m.right)
	}
	m.playback.finish()
}
