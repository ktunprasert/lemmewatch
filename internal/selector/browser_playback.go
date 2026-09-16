package selector

import (
	"context"
	"errors"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
)

const autoplayPrefetchLead = 60.0

type PlaybackStatus struct {
	Position  float64
	Duration  float64
	Completed bool
}

type playFinished struct {
	err    error
	id     uint64
	status PlaybackStatus
}

type playStatus struct {
	id     uint64
	status PlaybackStatus
}

type playProgress struct {
	id   toastID
	text string
}

type playbackModel struct {
	active  bool
	running bool
	id      uint64
	ctx     context.Context
	stop    context.CancelFunc
	ch      <-chan string
	status  <-chan PlaybackStatus
	toast   toastID
}

func (p *playbackModel) busy() bool { return p.active }

func (p *playbackModel) start(ctx context.Context, ch <-chan string, status <-chan PlaybackStatus, stop context.CancelFunc, toast toastID) uint64 {
	p.id++
	p.active = true
	p.running = true
	p.ctx = ctx
	p.stop = stop
	p.ch = ch
	p.status = status
	p.toast = toast
	return p.id
}

func (p *playbackModel) finish() {
	if p.stop != nil {
		p.stop()
	}
	p.active = false
	p.running = false
	p.ctx = nil
	p.stop = nil
	p.ch = nil
	p.status = nil
	p.toast = 0
}

func (p *playbackModel) stopPlayback() bool {
	if p.stop == nil {
		return false
	}
	p.stop()
	return true
}

type autoplayCursor[T item] struct {
	valid        bool
	seasonLevel  int
	seasons      []T
	seasonIndex  int
	episodes     []T
	episodeIndex int
}

type autoplayModel[T item] struct {
	playID           uint64
	cursor           autoplayCursor[T]
	next             *episodeSwitched[T]
	prefetching      bool
	prefetchProvider string
	completed        bool
}

type autoplayPrefetched[T item] struct {
	playID uint64
	next   episodeSwitched[T]
}

func (m *browserModel[T]) startPlayback(selected T) (tea.Model, tea.Cmd) {
	return m.startPlaybackWithCursor(selected, m.captureAutoplayCursor(), "Starting playback...")
}

func (m *browserModel[T]) startPlaybackWithCursor(selected T, cursor autoplayCursor[T], text string) (tea.Model, tea.Cmd) {
	playContext, cancel := context.WithCancel(m.ctx)
	queued := false
	if stream, ok := any(selected).(streamItem); ok {
		info, isStream := stream.StreamInfo()
		queued = isStream && info.CacheApplicable && !info.Playable
	}
	if queued {
		text = "Queueing uncached torrent; playback starts after download..."
	}
	var ch <-chan string
	if m.options.Progress != nil {
		ch = m.options.Progress()
	}
	statusCh := make(chan PlaybackStatus, 16)
	m.toasts.Spin(ch != nil)
	id := m.toasts.Set(ToastPlayback, "%s", text)
	playID := m.playback.start(playContext, ch, statusCh, cancel, id)
	m.autoplayPlayback = autoplayModel[T]{playID: playID, cursor: cursor}
	playCmd := func() tea.Msg {
		var mu sync.Mutex
		var last PlaybackStatus
		report := func(status PlaybackStatus) {
			mu.Lock()
			last = status
			mu.Unlock()
			select {
			case statusCh <- status:
			case <-playContext.Done():
			}
		}
		err := m.options.Play(playContext, selected, report)
		close(statusCh)
		mu.Lock()
		status := last
		mu.Unlock()
		return playFinished{err: err, id: playID, status: status}
	}
	commands := []tea.Cmd{playCmd, listenPlaybackStatus(statusCh, playID)}
	if ch != nil {
		commands = append(commands, listenProgress(ch, id))
	}
	return *m, tea.Batch(commands...)
}

func (m browserModel[T]) captureAutoplayCursor() autoplayCursor[T] {
	if !m.focusRight || !m.rightHasStreams() || len(m.levels) < 2 {
		return autoplayCursor[T]{}
	}
	episodes := m.filteredCurrent()
	seasonLevel := len(m.levels) - 2
	seasons := filterItems(m.levels[seasonLevel].items, m.levels[seasonLevel].filter)
	if len(episodes) == 0 || len(seasons) == 0 {
		return autoplayCursor[T]{}
	}
	episode := episodes[clamp(m.current().index, len(episodes))]
	season := seasons[clamp(m.levels[seasonLevel].index, len(seasons))]
	return autoplayCursor[T]{
		valid:        true,
		seasonLevel:  seasonLevel,
		seasons:      append([]T(nil), m.levels[seasonLevel].items...),
		seasonIndex:  season.index,
		episodes:     append([]T(nil), m.current().items...),
		episodeIndex: episode.index,
	}
}

func (m *browserModel[T]) updatePlaybackProgress(msg playProgress) (tea.Model, tea.Cmd) {
	if !m.playback.active || msg.id != m.playback.toast {
		return *m, nil
	}
	m.toasts.Update(msg.id, msg.text)
	return *m, listenProgress(m.playback.ch, msg.id)
}

func (m *browserModel[T]) updatePlaybackStatus(msg playStatus) (tea.Model, tea.Cmd) {
	if !m.playback.active || msg.id != m.playback.id {
		return *m, nil
	}
	if msg.status.Duration > 0 && msg.status.Position >= 0 {
		m.autoplayPlayback.completed = msg.status.Completed
	}
	var prefetch tea.Cmd
	if m.shouldPrefetchAutoplay(msg.status) {
		prefetch = m.startAutoplayPrefetch()
	}
	listen := listenPlaybackStatus(m.playback.status, msg.id)
	if prefetch != nil {
		return *m, tea.Batch(listen, prefetch)
	}
	return *m, listen
}

func (m browserModel[T]) shouldPrefetchAutoplay(status PlaybackStatus) bool {
	state := m.autoplayPlayback
	return m.autoplay && state.cursor.valid && !state.prefetching && state.next == nil &&
		status.Duration > 0 && status.Position >= 0 && status.Duration-status.Position <= autoplayPrefetchLead
}

func (m *browserModel[T]) startAutoplayPrefetch() tea.Cmd {
	m.autoplayPlayback.prefetching = true
	playID := m.autoplayPlayback.playID
	cursor := m.autoplayPlayback.cursor
	provider := m.provider
	m.autoplayPlayback.prefetchProvider = provider
	ctx := m.playback.ctx
	load := m.load
	childTitle := m.childTitle
	return func() tea.Msg {
		next := func(episodes []T, seasonIndex, episodeIndex int, season T) episodeSwitched[T] {
			episode := episodes[episodeIndex]
			streams, err := load(ctx, episode)
			if err != nil {
				return episodeSwitched[T]{provider: provider, direction: 1, err: err}
			}
			return episodeSwitched[T]{
				episodes: episodes, streams: streams, seasonIndex: seasonIndex, episodeIndex: episodeIndex,
				seasonLabel: plainLabel(season.Label()), episodeLabel: plainLabel(episode.Label()),
				title: childTitle(episode), key: itemCacheKey(episode, provider), provider: provider, direction: 1, found: true,
			}
		}
		season := cursor.seasons[cursor.seasonIndex]
		for index := cursor.episodeIndex + 1; index < len(cursor.episodes); index++ {
			if itemUnavailable(cursor.episodes[index]) {
				return autoplayPrefetched[T]{playID: playID, next: episodeSwitched[T]{provider: provider, direction: 1}}
			}
			return autoplayPrefetched[T]{playID: playID, next: next(cursor.episodes, cursor.seasonIndex, index, season)}
		}
		for seasonIndex := cursor.seasonIndex + 1; seasonIndex < len(cursor.seasons); seasonIndex++ {
			season = cursor.seasons[seasonIndex]
			episodes, err := load(ctx, season)
			if err != nil {
				return autoplayPrefetched[T]{playID: playID, next: episodeSwitched[T]{provider: provider, direction: 1, err: err}}
			}
			for episodeIndex, episode := range episodes {
				if itemUnavailable(episode) {
					return autoplayPrefetched[T]{playID: playID, next: episodeSwitched[T]{provider: provider, direction: 1}}
				}
				return autoplayPrefetched[T]{playID: playID, next: next(episodes, seasonIndex, episodeIndex, season)}
			}
		}
		return autoplayPrefetched[T]{playID: playID, next: episodeSwitched[T]{provider: provider, direction: 1}}
	}
}

func (m *browserModel[T]) updateAutoplayPrefetched(msg autoplayPrefetched[T]) (tea.Model, tea.Cmd) {
	if !m.playback.active || msg.playID != m.playback.id || msg.playID != m.autoplayPlayback.playID {
		return *m, nil
	}
	if msg.next.provider != m.autoplayPlayback.prefetchProvider {
		return *m, nil
	}
	if msg.next.provider != m.provider {
		m.autoplayPlayback.prefetching = false
		return *m, nil
	}
	m.autoplayPlayback.prefetching = false
	m.autoplayPlayback.next = &msg.next
	if !m.playback.running && m.autoplayPlayback.completed && m.autoplay {
		return m.startPrefetchedPlayback()
	}
	return *m, nil
}

func (m *browserModel[T]) updatePlaybackFinished(msg playFinished) (tea.Model, tea.Cmd) {
	if !m.playback.active || msg.id != m.playback.id {
		return *m, nil
	}
	m.playback.running = false
	if msg.status.Duration > 0 && msg.status.Position >= 0 {
		m.autoplayPlayback.completed = msg.status.Completed
	}
	m.toasts.Spin(false)
	switch {
	case errors.Is(msg.err, context.Canceled):
		m.playback.finish()
		m.toasts.Set(ToastPlayback, "Playback stopped")
		return *m, nil
	case msg.err != nil:
		m.playback.finish()
		m.toasts.Err(ToastPlayback, "Playback failed: %s", msg.err.Error())
		return *m, nil
	case !m.autoplay || !m.autoplayPlayback.cursor.valid || !m.autoplayPlayback.completed:
		m.playback.finish()
		m.toasts.Set(ToastPlayback, "Playback launched")
		return *m, nil
	case m.autoplayPlayback.next != nil:
		return m.startPrefetchedPlayback()
	case !m.autoplayPlayback.prefetching:
		m.toasts.Set(ToastPlayback, "Finding next episode...")
		return *m, m.startAutoplayPrefetch()
	default:
		m.toasts.Set(ToastPlayback, "Preparing next episode...")
		return *m, nil
	}
}

func (m *browserModel[T]) startPrefetchedPlayback() (tea.Model, tea.Cmd) {
	next := m.autoplayPlayback.next
	if next == nil {
		return *m, nil
	}
	if next.err != nil {
		m.playback.finish()
		m.toasts.Err(ToastPlayback, "Autoplay failed: %s", next.err.Error())
		return *m, nil
	}
	if !next.found {
		m.playback.finish()
		m.toasts.Set(ToastEpisode, "%s", episodeBoundaryNotice(1))
		return *m, nil
	}
	stream, streamIndex, ok := m.autoplayStream(next.streams)
	if !ok {
		m.playback.finish()
		m.toasts.Set(ToastPlayback, "Autoplay stopped: no matching playable stream")
		return *m, nil
	}
	cursor := autoplayCursor[T]{
		valid: true, seasonLevel: m.autoplayPlayback.cursor.seasonLevel,
		seasons: m.autoplayPlayback.cursor.seasons, seasonIndex: next.seasonIndex,
		episodes: next.episodes, episodeIndex: next.episodeIndex,
	}
	if m.applyAutoplayNavigation(*next, cursor) {
		m.right.index = streamIndex
	}
	m.markWatched(stream)
	m.playback.finish()
	return m.startPlaybackWithCursor(stream, cursor, "Starting next episode...")
}

func (m browserModel[T]) autoplayStream(streams []T) (T, int, bool) {
	copy := m
	copy.right = pane[T]{items: streams}
	for index, candidate := range copy.filteredRight() {
		stream, ok := any(candidate.item).(streamItem)
		if !ok {
			continue
		}
		info, isStream := stream.StreamInfo()
		if isStream && (info.Playable || info.CacheApplicable) {
			return candidate.item, index, true
		}
	}
	var zero T
	return zero, 0, false
}

func (m *browserModel[T]) applyAutoplayNavigation(next episodeSwitched[T], cursor autoplayCursor[T]) bool {
	if cursor.seasonLevel < 0 || cursor.seasonLevel+1 >= len(m.levels) || cursor.seasonIndex >= len(cursor.seasons) {
		return false
	}
	currentSeasons := m.levels[cursor.seasonLevel].items
	if cursor.seasonIndex >= len(currentSeasons) || !sameItem(currentSeasons[cursor.seasonIndex], cursor.seasons[cursor.seasonIndex]) {
		return false
	}
	m.levels[cursor.seasonLevel].index = cursor.seasonIndex
	m.levels[cursor.seasonLevel].filter = ""
	m.levels[cursor.seasonLevel+1].items = next.episodes
	m.levels[cursor.seasonLevel+1].index = next.episodeIndex
	m.levels[cursor.seasonLevel+1].filter = ""
	m.right = pane[T]{title: next.title, items: next.streams}
	m.focusRight = true
	m.err = nil
	if cursor.seasonLevel+1 < len(m.crumbs) {
		m.crumbs[cursor.seasonLevel] = next.seasonLabel
		m.crumbs[cursor.seasonLevel+1] = next.episodeLabel
	}
	if next.key != "" {
		if m.loadCache == nil {
			m.loadCache = make(map[string][]T)
		}
		m.loadCache[next.key] = next.streams
	}
	return true
}

func sameItem[T item](left, right T) bool {
	leftKey, rightKey := itemCacheKey(left, ""), itemCacheKey(right, "")
	if leftKey != "" || rightKey != "" {
		return leftKey != "" && leftKey == rightKey
	}
	return left.Label() == right.Label()
}

func (m *browserModel[T]) markWatched(selected T) {
	watchable, ok := any(selected).(watchableItem)
	if !ok {
		return
	}
	identity, keys := watchable.WatchIdentity()
	if identity == "" {
		return
	}
	if m.options.Watched == nil {
		m.options.Watched = make(map[string]bool)
	}
	m.options.Watched[identity] = true
	for _, key := range keys {
		m.options.Watched[identity+":"+key] = true
	}
}

func listenProgress(ch <-chan string, id toastID) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok {
			return nil
		}
		return playProgress{id: id, text: text}
	}
}

func listenPlaybackStatus(ch <-chan PlaybackStatus, id uint64) tea.Cmd {
	return func() tea.Msg {
		status, ok := <-ch
		if !ok {
			return nil
		}
		return playStatus{id: id, status: status}
	}
}
