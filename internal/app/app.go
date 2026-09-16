package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/config"
	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/provider"
	"lemmewatch/internal/selector"
	"lemmewatch/internal/storage"
	"lemmewatch/internal/torbox"
)

type App struct {
	Catalog          catalog.Client
	Version          string
	Providers        map[string]provider.Provider
	ProvidersMu      *sync.RWMutex
	ProviderNames    []string
	Provider         string
	ProviderError    error
	TorBox           torbox.Client
	Player           player.Player
	DefaultPlayer    player.Player
	PlayerOverridden bool
	Storage          *storage.Storage
	In               io.Reader
	Out              io.Writer
	Err              io.Writer
}

func (a App) ValidateProvider() error {
	if a.ProviderError != nil {
		return a.ProviderError
	}
	if a.Provider == provider.TorBoxID && a.TorBox.Token == "" {
		return fmt.Errorf("TORBOX_API_TOKEN is required")
	}
	_, err := a.provider(a.Provider)
	return err
}

type navigationKind int

func (a App) Dashboard(ctx context.Context, input io.Reader, output io.Writer) error {
	preferences := config.Load()
	result, err := selector.Dashboard(ctx, input, output, selector.DashboardOptions{Groups: []string{string(model.Movie), string(model.Series)}, PreferredGroup: preferences.MediaTab, Version: a.Version})
	if err != nil {
		return err
	}
	a.In, a.Out = input, output
	switch result.Action {
	case selector.DashboardSearch:
		preferences.MediaTab = result.Group
		if err := config.Save(preferences); err != nil {
			return fmt.Errorf("save media tab preference: %w", err)
		}
		return a.Watch(ctx, result.Query)
	case selector.DashboardHistory:
		return a.History(ctx)
	default:
		return nil
	}
}

func applyPlayerPreference(active *player.Player, fallback player.Player, overridden bool, preference string) error {
	executable, arguments, err := player.ParseCommand(preference)
	if preference == "" {
		executable, arguments, err = fallback.Executable, fallback.Arguments, nil
	}
	if err != nil {
		return err
	}
	if overridden {
		return nil
	}
	*active = fallback
	active.Executable = executable
	active.Arguments = append([]string(nil), arguments...)
	return nil
}

const (
	navigationMedia navigationKind = iota
	navigationSeason
	navigationEpisode
	navigationStream
)

const (
	searchCacheTTL = 24 * time.Hour
	seriesCacheTTL = 30 * 24 * time.Hour
)

type navigationChoice struct {
	kind     navigationKind
	media    model.Media
	playedAt time.Time
	season   int
	episodes []model.Episode
	episode  model.Episode
	stream   model.Stream
}

func (n navigationChoice) Label() string {
	switch n.kind {
	case navigationMedia:
		return n.media.Name
	case navigationSeason:
		return fmt.Sprintf("Season %d", n.season)
	case navigationEpisode:
		return fmt.Sprintf("Episode %d  %s", n.episode.Episode, n.episode.Title)
	case navigationStream:
		return n.stream.Title
	default:
		return "Unknown"
	}
}

// RowLabel keeps the navigation identifier intact when the title is truncated.
func (n navigationChoice) RowLabel() (prefix, title string) {
	switch n.kind {
	case navigationSeason:
		return n.Label(), ""
	case navigationEpisode:
		return fmt.Sprintf("Episode %d", n.episode.Episode), n.episode.Title
	default:
		return "", n.Label()
	}
}

func (n navigationChoice) ContextModes() []selector.ContextMode {
	switch n.kind {
	case navigationMedia:
		year := ""
		if n.media.Year > 0 {
			year = strconv.Itoa(n.media.Year)
		}
		modes := []selector.ContextMode{{Group: "media", Key: "y", Name: "Year", Value: year}, {Group: "media", Key: "r", Name: "Rating", Value: ratingLabel(n.media.Rating)}, {Group: "media", Key: "i", Name: "ID", Value: n.media.ID}, {Group: "media", Key: "t", Name: "Type", Value: string(n.media.Type)}}
		if !n.playedAt.IsZero() {
			modes = append(modes, selector.ContextMode{Group: "media", Key: "p", Name: "Date played", Value: n.playedAt.Local().Format("2006-01-02")})
		}
		return modes
	case navigationSeason:
		return []selector.ContextMode{{Group: "season", Key: "e", Name: "Episodes", Value: fmt.Sprintf("%d episodes", len(n.episodes))}}
	case navigationEpisode:
		date := ""
		if !n.episode.Released.IsZero() {
			date = n.episode.Released.Format("2006-01-02")
		}
		rating := ratingLabel(n.episode.Rating)
		if rating == "--" && ratingLabel(n.media.Rating) != "--" {
			rating = n.media.Rating + " show"
		}
		return []selector.ContextMode{{Group: "episode", Key: "a", Name: "Air date", Value: date}, {Group: "episode", Key: "r", Name: "Rating", Value: rating}, {Group: "episode", Key: "i", Name: "ID", Value: n.episode.ID}}
	case navigationStream:
		quality := ""
		if n.stream.Quality > 0 {
			quality = fmt.Sprintf("%dp", n.stream.Quality)
		}
		cached := "direct"
		if n.stream.Cache == model.CacheCached {
			cached = "cached"
		} else if n.stream.Cache == model.CacheUncached {
			cached = "uncached"
		}
		return []selector.ContextMode{
			{Group: "stream", Key: "q", Name: "Quality", Value: quality}, {Group: "stream", Key: "c", Name: "Availability", Value: cached},
			{Group: "stream", Key: "z", Name: "Size", Value: formatSize(n.stream.Size)}, {Group: "stream", Key: "s", Name: "Seeders", Value: strconv.Itoa(n.stream.Seeders)},
			{Group: "stream", Key: "o", Name: "Source", Value: n.stream.Source}, {Group: "stream", Key: "f", Name: "Filename", Value: n.stream.Filename},
		}
	}
	return nil
}

func formatSize(size int64) string {
	if size <= 0 {
		return ""
	}
	return fmt.Sprintf("%.2f GB", float64(size)/1e9)
}

func (n navigationChoice) Group() string {
	if n.kind == navigationMedia {
		return string(n.media.Type)
	}
	return ""
}
func (n navigationChoice) Terminal() bool { return n.kind == navigationStream }
func (n navigationChoice) MetadataRoot() bool {
	return n.kind == navigationMedia && n.media.Type == model.Series
}
func (n navigationChoice) Unavailable() bool {
	return n.kind == navigationEpisode && !n.episode.Released.IsZero() && n.episode.Released.After(time.Now())
}
func (n navigationChoice) CacheKey() string {
	switch n.kind {
	case navigationMedia:
		if n.media.Type == model.Series {
			return "series:" + n.media.ID
		}
		return "streams:" + n.media.ID
	case navigationEpisode:
		return "streams:" + n.episode.ID
	case navigationSeason:
		return fmt.Sprintf("season:%s:%d", n.media.ID, n.season)
	}
	return ""
}
func (n navigationChoice) WatchIdentity() (string, []string) {
	if n.media.ID == "" {
		return "", nil
	}
	switch n.kind {
	case navigationEpisode, navigationStream:
		if n.episode.ID == "" {
			return n.media.ID, nil
		}
		return n.media.ID, []string{fmt.Sprintf("%d:%d", n.episode.Season, n.episode.Episode)}
	case navigationSeason:
		keys := make([]string, len(n.episodes))
		for i, episode := range n.episodes {
			keys[i] = fmt.Sprintf("%d:%d", episode.Season, episode.Episode)
		}
		return n.media.ID, keys
	default:
		return n.media.ID, nil
	}
}
func (n navigationChoice) WatchThrough() bool {
	return n.kind == navigationSeason || n.kind == navigationEpisode
}
func (n navigationChoice) StreamInfo() (selector.StreamInfo, bool) {
	return selector.StreamInfo{Cached: n.stream.Cache == model.CacheCached, CacheApplicable: n.stream.Cache != model.CacheNotApplicable, Playable: n.stream.Playable, Quality: n.stream.Quality, AudioLanguages: n.stream.AudioLanguages, SubtitleLanguages: n.stream.SubtitleLanguages, LanguageHints: n.stream.LanguageHints, MatchRank: n.stream.MatchRank}, n.kind == navigationStream
}
func (n navigationChoice) SortFields() (string, int, time.Time, bool) {
	if n.kind == navigationStream {
		return n.stream.Title, 0, time.Time{}, true
	}
	return n.media.Name, n.media.Year, n.playedAt, n.kind == navigationMedia
}

func (n navigationChoice) Status(watched map[string]bool) string {
	if n.kind == navigationSeason {
		seen := 0
		for _, episode := range n.episodes {
			if watched[fmt.Sprintf("%s:%d:%d", n.media.ID, episode.Season, episode.Episode)] {
				seen++
			}
		}
		if seen > 0 && seen < len(n.episodes) {
			return "~"
		}
	}
	if n.kind == navigationMedia && n.media.UpdateEpisode != "" && !watched[n.media.ID+":"+n.media.UpdateEpisode] {
		return "+"
	}
	return ""
}

func (a App) Search(ctx context.Context, query string, kind model.MediaType) ([]model.Media, error) {
	fmt.Fprintf(a.Err, "Searching catalog for %q...\n", query)
	return a.searchCatalog(ctx, query, kind)
}

func (a App) searchCatalog(ctx context.Context, query string, kind model.MediaType) ([]model.Media, error) {
	if kind != "" {
		return a.searchCatalogType(ctx, query, kind)
	}
	type result struct {
		items []model.Media
		err   error
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for _, mediaType := range []model.MediaType{model.Movie, model.Series} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			items, err := a.searchCatalogType(ctx, query, mediaType)
			results <- result{items, err}
		}()
	}
	go func() { wg.Wait(); close(results) }()
	var items []model.Media
	var firstErr error
	seen := make(map[string]bool)
	for r := range results {
		if r.err != nil {
			if firstErr == nil {
				firstErr = r.err
			}
			continue
		}
		for _, item := range r.items {
			key := string(item.Type) + ":" + item.ID
			if !seen[key] {
				seen[key] = true
				items = append(items, item)
			}
		}
	}
	if len(items) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return items, nil
}

func (a App) searchCatalogType(ctx context.Context, query string, kind model.MediaType) ([]model.Media, error) {
	key := "v2:" + storage.SourceFingerprint(a.Catalog.BaseURL) + ":" + string(kind) + ":" + strings.ToLower(strings.TrimSpace(query))
	var items []model.Media
	if a.Storage != nil {
		if hit, _ := a.Storage.CacheGet(storage.CacheSearch, key, &items); hit {
			return items, nil
		}
	}
	items, err := a.Catalog.Search(ctx, kind, query)
	if err == nil && a.Storage != nil {
		_ = a.Storage.CachePut(storage.CacheSearch, key, items, searchCacheTTL)
	}
	return items, err
}

func (a App) seriesEpisodes(ctx context.Context, imdbID string, refresh bool) ([]model.Episode, error) {
	details, err := a.catalogDetails(ctx, model.Series, imdbID, refresh)
	return details.Episodes, err
}

func (a App) seriesCacheKey(imdbID string) string {
	return storage.SourceFingerprint(a.Catalog.BaseURL) + ":" + imdbID
}

func (a App) LookupStreams(ctx context.Context, imdbID string) ([]model.Stream, error) {
	fmt.Fprintf(a.Err, "Querying %s...\n", a.Provider)
	selected, err := a.provider(a.Provider)
	if err != nil {
		return nil, err
	}
	return selected.Streams(ctx, provider.Request{MediaType: model.Movie, ID: imdbID})
}

func (a App) provider(id string) (provider.Provider, error) {
	if a.ProvidersMu != nil {
		a.ProvidersMu.RLock()
		defer a.ProvidersMu.RUnlock()
	}
	selected := a.Providers[id]
	if selected == nil {
		return nil, fmt.Errorf("unknown provider %q", id)
	}
	return selected, nil
}

func (a *App) setTorBoxToken(token string) error {
	if a.ProvidersMu != nil {
		a.ProvidersMu.Lock()
		defer a.ProvidersMu.Unlock()
	}
	selected, ok := a.Providers[provider.TorBoxID].(provider.TorBox)
	if !ok {
		return fmt.Errorf("TorBox provider is unavailable")
	}
	a.TorBox.Token = token
	selected.TorBoxClient.Token = token
	a.Providers[provider.TorBoxID] = selected
	return nil
}

func (a App) Cache(ctx context.Context, hashes []string) (map[string]bool, error) {
	if a.TorBox.Token == "" {
		return nil, fmt.Errorf("TORBOX_API_TOKEN is required")
	}
	fmt.Fprintf(a.Err, "Checking TorBox cache (%d candidates)...\n", len(hashes))
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	return a.TorBox.Cached(ctx, hashes)
}

func (a App) Watch(ctx context.Context, query string) error {
	return a.browseMedia(ctx, nil, "Search results", query, []string{string(model.Movie), string(model.Series)}, true)
}

func (a App) History(ctx context.Context) error {
	items, err := a.loadHistoryMedia()
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	return a.browseMedia(ctx, items, "History", "History", nil, false)
}

func (a App) loadHistoryMedia() ([]model.Media, error) {
	entries, err := a.Storage.History()
	if err != nil {
		return nil, err
	}
	items := historyMedia(entries)
	byID := make(map[string]storage.HistoryEntry, len(entries))
	for _, entry := range entries {
		byID[entry.ID] = entry
	}
	now := time.Now()
	for i := range items {
		if items[i].Type != model.Series {
			continue
		}
		entry := byID[items[i].ID]
		if len(entry.Episodes) == 0 {
			continue
		}
		var episodes []model.Episode
		if hit, _ := a.Storage.CacheGet(storage.CacheSeries, a.seriesCacheKey(items[i].ID), &episodes); !hit {
			continue
		}
		latest, ok := possibleEpisodeUpdate(entry.Episodes, episodes, now)
		if ok {
			items[i].UpdateEpisode = fmt.Sprintf("%d:%d", latest.Season, latest.Episode)
		}
	}
	return items, nil
}

type episodePosition struct {
	season  int
	episode int
}

func possibleEpisodeUpdate(watchedKeys []string, episodes []model.Episode, now time.Time) (model.Episode, bool) {
	var watched episodePosition
	hasWatched := false
	for _, key := range watchedKeys {
		position, ok := parseEpisodePosition(key)
		if ok && (!hasWatched || position.after(watched)) {
			watched = position
			hasWatched = true
		}
	}
	if !hasWatched {
		return model.Episode{}, false
	}

	var latest model.Episode
	hasLatest := false
	for _, episode := range episodes {
		if episode.Season <= 0 || episode.Episode <= 0 || episode.Released.IsZero() || episode.Released.After(now) {
			continue
		}
		position := episodePosition{season: episode.Season, episode: episode.Episode}
		if !hasLatest || position.after(episodePosition{season: latest.Season, episode: latest.Episode}) {
			latest = episode
			hasLatest = true
		}
	}
	return latest, hasLatest && (episodePosition{season: latest.Season, episode: latest.Episode}).after(watched)
}

func parseEpisodePosition(key string) (episodePosition, bool) {
	seasonText, episodeText, ok := strings.Cut(key, ":")
	if !ok || strings.Contains(episodeText, ":") {
		return episodePosition{}, false
	}
	season, seasonErr := strconv.Atoi(seasonText)
	episode, episodeErr := strconv.Atoi(episodeText)
	if seasonErr != nil || episodeErr != nil || season <= 0 || episode <= 0 {
		return episodePosition{}, false
	}
	return episodePosition{season: season, episode: episode}, true
}

func (p episodePosition) after(other episodePosition) bool {
	return p.season > other.season || p.season == other.season && p.episode > other.episode
}

func historyMedia(entries []storage.HistoryEntry) []model.Media {
	items := make([]model.Media, 0, len(entries))
	for _, entry := range entries {
		mediaType := model.MediaType(entry.Type)
		if mediaType != model.Movie && mediaType != model.Series {
			continue
		}
		items = append(items, model.Media{ID: entry.ID, Type: mediaType, Name: entry.Title, PlayedAt: entry.PlayedAt})
	}
	return items
}

func (a App) browseMedia(ctx context.Context, items []model.Media, initialTitle, initialQuery string, parentGroups []string, initialSearch bool) error {
	watched, err := a.Storage.Watched()
	if err != nil {
		return fmt.Errorf("read watched state: %w", err)
	}
	choices := make([]navigationChoice, len(items))
	for i, item := range items {
		choices[i] = navigationChoice{kind: navigationMedia, media: item, playedAt: item.PlayedAt}
	}
	preferences := config.Load()
	providerID := a.Provider
	var playbackMu sync.RWMutex
	var progressCh chan string
	requery := func(searchContext context.Context, query string) ([]navigationChoice, error) {
		results, err := a.searchCatalog(searchContext, query, "")
		if err != nil {
			return nil, err
		}
		choices := make([]navigationChoice, len(results))
		for i, result := range results {
			choices[i] = navigationChoice{kind: navigationMedia, media: result}
		}
		return choices, nil
	}
	load := func(ctx context.Context, selected navigationChoice, refresh bool) ([]navigationChoice, error) {
		switch selected.kind {
		case navigationMedia:
			selectedProvider, err := a.provider(providerID)
			if err != nil {
				return nil, err
			}
			if selected.media.Type == model.Movie {
				streams, streamErr := selectedProvider.Streams(ctx, provider.Request{MediaType: model.Movie, ID: selected.media.ID, Title: selected.media.Name, Refresh: refresh})
				return streamChoices(selected.media, model.Episode{}, streams, streamErr)
			}
			details, err := a.catalogDetails(ctx, model.Series, selected.media.ID, refresh)
			if err != nil {
				return nil, err
			}
			selected.media = mergeMediaDetails(selected.media, details.Media)
			episodes := details.Episodes
			bySeason := make(map[int][]model.Episode)
			for _, episode := range episodes {
				bySeason[episode.Season] = append(bySeason[episode.Season], episode)
			}
			for season := range bySeason {
				sort.SliceStable(bySeason[season], func(i, j int) bool {
					return bySeason[season][i].Episode < bySeason[season][j].Episode
				})
			}
			seasons := make([]int, 0, len(bySeason))
			for season := range bySeason {
				seasons = append(seasons, season)
			}
			sort.Ints(seasons)
			result := make([]navigationChoice, len(seasons))
			for i, season := range seasons {
				result[i] = navigationChoice{kind: navigationSeason, media: selected.media, season: season, episodes: bySeason[season]}
			}
			return result, nil
		case navigationSeason:
			result := make([]navigationChoice, len(selected.episodes))
			for i, episode := range selected.episodes {
				result[i] = navigationChoice{kind: navigationEpisode, media: selected.media, episode: episode}
			}
			return result, nil
		case navigationEpisode:
			selectedProvider, err := a.provider(providerID)
			if err != nil {
				return nil, err
			}
			streams, streamErr := selectedProvider.Streams(ctx, provider.Request{MediaType: model.Series, ID: selected.episode.ID, Title: selected.media.Name, Season: selected.episode.Season, Episode: selected.episode.Episode, Refresh: refresh})
			return streamChoices(selected.media, selected.episode, streams, streamErr)
		default:
			return nil, fmt.Errorf("item cannot be opened")
		}
	}
	_, err = selector.Browse(ctx, a.In, a.Out, choices, func(ctx context.Context, selected navigationChoice) ([]navigationChoice, error) {
		return load(ctx, selected, false)
	}, selector.BrowserOptions[navigationChoice]{
		InitialTitle:      initialTitle,
		InitialQuery:      initialQuery,
		InitialSearch:     initialSearch,
		Version:           a.Version,
		ParentGroups:      parentGroups,
		SearchGroups:      []string{string(model.Movie), string(model.Series)},
		PreferredGroup:    preferences.MediaTab,
		PreferredQuality:  preferences.Quality,
		PreferredCached:   preferences.CachedOnly,
		PreferredProvider: providerID,
		PreferredPlayer:   preferences.Player,
		PreferredPlayback: preferences.PlaybackPreferences,
		LoadInfo:          a.navigationInfo,
		SavePlayback: func(value model.PlaybackPreferences) error {
			next := preferences
			next.PlaybackPreferences = value
			if err := config.Save(next); err != nil {
				return err
			}
			preferences = next
			return nil
		},
		Providers:          a.ProviderNames,
		PreferredModes:     preferences.DetailModes,
		PreferredPaneSizes: preferences.PaneSizes,
		ModeOptions: map[string][]selector.ContextMode{
			"media":   {{Key: "y", Name: "Year"}, {Key: "r", Name: "Rating"}, {Key: "i", Name: "ID"}, {Key: "t", Name: "Type"}},
			"season":  {{Key: "e", Name: "Episodes"}},
			"episode": {{Key: "a", Name: "Air date"}, {Key: "r", Name: "Rating"}, {Key: "i", Name: "ID"}},
			"stream":  {{Key: "q", Name: "Quality"}, {Key: "c", Name: "Availability"}, {Key: "z", Name: "Size"}, {Key: "s", Name: "Seeders"}, {Key: "o", Name: "Source"}, {Key: "f", Name: "Filename"}},
		},
		ChildTitle: func(selected navigationChoice) string {
			switch selected.kind {
			case navigationMedia:
				if selected.media.Type == model.Series {
					return "Seasons"
				}
				return "Streams"
			case navigationSeason:
				return "Episodes"
			default:
				return "Streams"
			}
		},
		Refresh: func(ctx context.Context, selected navigationChoice) ([]navigationChoice, error) {
			return load(ctx, selected, true)
		},
		SaveGroup: func(group string) error {
			preferences.MediaTab = group
			return config.Save(preferences)
		},
		SaveQuality: func(quality int) error {
			preferences.Quality = quality
			return config.Save(preferences)
		},
		SaveCached: func(cachedOnly bool) error {
			preferences.CachedOnly = &cachedOnly
			return config.Save(preferences)
		},
		SaveProvider: func(selected string) error {
			if _, err := a.provider(selected); err != nil {
				return err
			}
			providerID = selected
			preferences.Provider = selected
			return config.Save(preferences)
		},
		ProviderNeedsAPIKey: func(selected string) bool {
			return selected == provider.TorBoxID && a.TorBox.Token == ""
		},
		SaveProviderAPIKey: func(selected, key string) error {
			if selected != provider.TorBoxID {
				return fmt.Errorf("API key is unsupported for provider %q", selected)
			}
			if _, err := a.provider(provider.TorBoxID); err != nil {
				return err
			}
			next := preferences
			next.Provider = selected
			next.TorBoxToken = key
			if err := config.Save(next); err != nil {
				return err
			}
			preferences = next
			providerID = selected
			return a.setTorBoxToken(key)
		},
		SavePlayer: func(player string) error {
			playbackMu.Lock()
			defer playbackMu.Unlock()
			nextPlayer := a.Player
			if err := applyPlayerPreference(&nextPlayer, a.DefaultPlayer, a.PlayerOverridden, player); err != nil {
				_ = config.LogFailure("player preference", err)
				return err
			}
			preferences.Player = player
			if err := config.Save(preferences); err != nil {
				return err
			}
			a.Player = nextPlayer
			return nil
		},
		SaveMode: func(group, mode string) error {
			if preferences.DetailModes == nil {
				preferences.DetailModes = make(map[string]string)
			}
			preferences.DetailModes[group] = mode
			return config.Save(preferences)
		},
		SavePaneSizes: func(count int, sizes []int) error {
			if preferences.PaneSizes == nil {
				preferences.PaneSizes = make(map[int][]int)
			}
			preferences.PaneSizes[count] = sizes
			return config.Save(preferences)
		},
		Progress: func() <-chan string {
			progressCh = make(chan string, 16)
			return progressCh
		},
		Play: func(playContext context.Context, selected navigationChoice) error {
			playbackMu.RLock()
			selectedPlayer := a.resumePlayer(selected)
			playbackMu.RUnlock()
			selectedPlayer.Preferences = config.Load().PlaybackPreferences
			ch := progressCh
			defer close(ch)
			streamProvider, err := a.provider(selected.stream.Provider)
			if err != nil {
				return err
			}
			if queuer, ok := streamProvider.(provider.QueueTorrenter); ok && selected.stream.Cache == model.CacheUncached {
				queueContext, cancel := context.WithTimeout(playContext, 30*time.Minute)
				defer cancel()
				ch <- "Queueing torrent..."
				if err := queuer.QueueTorrent(queueContext, selected.stream, func(p provider.QueueProgress) {
					if p.Progress > 0 {
						ch <- fmt.Sprintf("Downloading torrent: %.0f%%", p.Progress*100)
					}
				}); err != nil {
					return err
				}
				ch <- "Torrent downloaded; starting playback..."
			}
			playback, err := streamProvider.Resolve(playContext, selected.stream)
			if err != nil {
				return err
			}
			entry := storage.HistoryEntry{ID: selected.media.ID, Title: selected.media.Name, Type: string(selected.media.Type)}
			if selected.episode.ID != "" {
				entry.Episodes = []string{fmt.Sprintf("%d:%d", selected.episode.Season, selected.episode.Episode)}
			}
			if err := a.Storage.RecordHistory(entry); err != nil {
				return fmt.Errorf("record history: %w", err)
			}
			if err := selectedPlayer.Play(playContext, playback); err != nil {
				if playContext.Err() != nil {
					return playContext.Err()
				}
				_ = config.LogFailure("player", err)
				return err
			}
			return nil
		},
		Requery: requery,
		History: func(context.Context) ([]navigationChoice, error) {
			media, err := a.loadHistoryMedia()
			if err != nil {
				return nil, err
			}
			choices := make([]navigationChoice, len(media))
			for i, item := range media {
				choices[i] = navigationChoice{kind: navigationMedia, media: item, playedAt: item.PlayedAt}
			}
			return choices, nil
		},
		Watched: watched,
		ToggleWatched: func(_ context.Context, selected navigationChoice) (map[string]bool, error) {
			entry, keys, err := historySelection([]navigationChoice{selected})
			if err != nil {
				return nil, err
			}
			state, err := a.Storage.ToggleWatched(entry, keys)
			return map[string]bool(state), err
		},
		ToggleWatchedThrough: func(_ context.Context, selected []navigationChoice) (map[string]bool, error) {
			entry, keys, err := historySelection(selected)
			if err != nil {
				return nil, err
			}
			state, err := a.Storage.ToggleWatched(entry, keys)
			return map[string]bool(state), err
		},
		RemoveHistory: func(_ context.Context, selected navigationChoice) error {
			return a.Storage.RemoveHistory(selected.media.ID)
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func historySelection(selected []navigationChoice) (storage.HistoryEntry, []string, error) {
	if len(selected) == 0 {
		return storage.HistoryEntry{}, nil, fmt.Errorf("no watched items selected")
	}
	last := selected[len(selected)-1]
	entry := storage.HistoryEntry{ID: last.media.ID, Title: last.media.Name, Type: string(last.media.Type)}
	if entry.ID == "" {
		return storage.HistoryEntry{}, nil, fmt.Errorf("watched item has no media identity")
	}
	seen := make(map[string]bool)
	var keys []string
	for _, item := range selected {
		identity, itemKeys := item.WatchIdentity()
		if identity != entry.ID {
			return storage.HistoryEntry{}, nil, fmt.Errorf("watched items have different media identities")
		}
		for _, key := range itemKeys {
			if !seen[key] {
				seen[key] = true
				keys = append(keys, key)
			}
		}
	}
	return entry, keys, nil
}

func streamChoices(media model.Media, episode model.Episode, streams []model.Stream, streamErr error) ([]navigationChoice, error) {
	if streamErr != nil {
		return nil, streamErr
	}
	if len(streams) == 0 {
		return nil, fmt.Errorf("no playable streams found")
	}
	result := make([]navigationChoice, len(streams))
	for i, stream := range streams {
		result[i] = navigationChoice{kind: navigationStream, media: media, episode: episode, stream: stream}
	}
	return result, nil
}
