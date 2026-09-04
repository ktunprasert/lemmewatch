package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"sync"
	"time"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/config"
	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/provider"
	"lemmewatch/internal/selector"
	"lemmewatch/internal/torbox"
)

type App struct {
	Catalog          catalog.Client
	Providers        map[string]provider.Provider
	ProvidersMu      *sync.RWMutex
	ProviderNames    []string
	Provider         string
	ProviderError    error
	TorBox           torbox.Client
	Player           player.Player
	DefaultPlayer    player.Player
	PlayerOverridden bool
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
	result, err := selector.Dashboard(ctx, input, output, selector.DashboardOptions{Groups: []string{string(model.Movie), string(model.Series)}, PreferredGroup: preferences.MediaTab})
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

type navigationChoice struct {
	kind     navigationKind
	media    model.Media
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

func (n navigationChoice) ContextModes() []selector.ContextMode {
	switch n.kind {
	case navigationMedia:
		year := ""
		if n.media.Year > 0 {
			year = strconv.Itoa(n.media.Year)
		}
		return []selector.ContextMode{{Group: "media", Key: "y", Name: "Year", Value: year}, {Group: "media", Key: "r", Name: "Rating", Value: n.media.Rating}, {Group: "media", Key: "i", Name: "ID", Value: n.media.ID}, {Group: "media", Key: "t", Name: "Type", Value: string(n.media.Type)}}
	case navigationSeason:
		return []selector.ContextMode{{Group: "season", Key: "e", Name: "Episodes", Value: fmt.Sprintf("%d episodes", len(n.episodes))}}
	case navigationEpisode:
		date := ""
		if !n.episode.Released.IsZero() {
			date = n.episode.Released.Format("2006-01-02")
		}
		return []selector.ContextMode{{Group: "episode", Key: "a", Name: "Air date", Value: date}, {Group: "episode", Key: "r", Name: "Rating", Value: n.episode.Rating}, {Group: "episode", Key: "i", Name: "ID", Value: n.episode.ID}}
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
func (n navigationChoice) Unavailable() bool {
	return n.kind == navigationEpisode && !n.episode.Released.IsZero() && n.episode.Released.After(time.Now())
}
func (n navigationChoice) CacheKey() string {
	if n.kind == navigationEpisode {
		return "streams:" + n.episode.ID
	}
	return ""
}
func (n navigationChoice) StreamInfo() (selector.StreamInfo, bool) {
	return selector.StreamInfo{Cached: n.stream.Cache == model.CacheCached, CacheApplicable: n.stream.Cache != model.CacheNotApplicable, Playable: n.stream.Playable, Quality: n.stream.Quality}, n.kind == navigationStream
}
func (n navigationChoice) SortFields() (string, int, bool) {
	if n.kind == navigationStream {
		return n.stream.Title, 0, true
	}
	return n.media.Name, n.media.Year, n.kind == navigationMedia
}

func (a App) Search(ctx context.Context, query string, kind model.MediaType) ([]model.Media, error) {
	fmt.Fprintf(a.Err, "Searching catalog for %q...\n", query)
	return a.searchCatalog(ctx, query, kind)
}

func (a App) searchCatalog(ctx context.Context, query string, kind model.MediaType) ([]model.Media, error) {
	if kind != "" {
		return a.Catalog.Search(ctx, kind, query)
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
			items, err := a.Catalog.Search(ctx, mediaType, query)
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
	items, err := a.Search(ctx, query, "")
	if err != nil {
		return err
	}
	return a.browseMedia(ctx, items, "Search results", query, []string{string(model.Movie), string(model.Series)})
}

func (a App) History(ctx context.Context) error {
	items, err := loadHistoryMedia()
	if err != nil {
		return fmt.Errorf("read history: %w", err)
	}
	return a.browseMedia(ctx, items, "History", "History", nil)
}

func loadHistoryMedia() ([]model.Media, error) {
	entries, err := config.History()
	if err != nil {
		return nil, err
	}
	return historyMedia(entries), nil
}

func historyMedia(entries []config.HistoryEntry) []model.Media {
	items := make([]model.Media, 0, len(entries))
	for _, entry := range entries {
		mediaType := model.MediaType(entry.Type)
		if mediaType != model.Movie && mediaType != model.Series {
			continue
		}
		items = append(items, model.Media{ID: entry.ID, Type: mediaType, Name: entry.Title})
	}
	return items
}

func (a App) browseMedia(ctx context.Context, items []model.Media, initialTitle, initialQuery string, parentGroups []string) error {
	choices := make([]navigationChoice, len(items))
	for i, item := range items {
		choices[i] = navigationChoice{kind: navigationMedia, media: item}
	}
	preferences := config.Load()
	providerID := a.Provider
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
	_, err := selector.Browse(ctx, a.In, a.Out, choices, func(ctx context.Context, selected navigationChoice) ([]navigationChoice, error) {
		switch selected.kind {
		case navigationMedia:
			selectedProvider, err := a.provider(providerID)
			if err != nil {
				return nil, err
			}
			if selected.media.Type == model.Movie {
				streams, streamErr := selectedProvider.Streams(ctx, provider.Request{MediaType: model.Movie, ID: selected.media.ID, Title: selected.media.Name})
				return streamChoices(selected.media, streams, streamErr)
			}
			episodes, err := a.Catalog.Episodes(ctx, selected.media.ID)
			if err != nil {
				return nil, err
			}
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
			streams, streamErr := selectedProvider.Streams(ctx, provider.Request{MediaType: model.Series, ID: selected.episode.ID, Title: selected.media.Name, Season: selected.episode.Season, Episode: selected.episode.Episode})
			return streamChoices(selected.media, streams, streamErr)
		default:
			return nil, fmt.Errorf("item cannot be opened")
		}
	}, selector.BrowserOptions[navigationChoice]{
		InitialTitle:      initialTitle,
		InitialQuery:      initialQuery,
		ParentGroups:      parentGroups,
		SearchGroups:      []string{string(model.Movie), string(model.Series)},
		PreferredGroup:    preferences.MediaTab,
		PreferredQuality:  preferences.Quality,
		PreferredCached:   preferences.CachedOnly,
		PreferredProvider: providerID,
		PreferredPlayer:   preferences.Player,
		Providers:         a.ProviderNames,
		PreferredModes:    preferences.DetailModes,
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
		Play: func(playContext context.Context, selected navigationChoice) error {
			streamProvider, err := a.provider(selected.stream.Provider)
			if err != nil {
				return err
			}
			playback, err := streamProvider.Resolve(playContext, selected.stream)
			if err != nil {
				return err
			}
			if err := config.RecordHistory(config.HistoryEntry{ID: selected.media.ID, Title: selected.media.Name, Type: string(selected.media.Type)}); err != nil {
				return fmt.Errorf("record history: %w", err)
			}
			if err := a.Player.Play(playContext, playback); err != nil {
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
			media, err := loadHistoryMedia()
			if err != nil {
				return nil, err
			}
			choices := make([]navigationChoice, len(media))
			for i, item := range media {
				choices[i] = navigationChoice{kind: navigationMedia, media: item}
			}
			return choices, nil
		},
		ToggleHistory: func(_ context.Context, selected navigationChoice) (bool, error) {
			return config.ToggleHistory(config.HistoryEntry{ID: selected.media.ID, Title: selected.media.Name, Type: string(selected.media.Type)})
		},
		RemoveHistory: func(_ context.Context, selected navigationChoice) error {
			return config.RemoveHistory(selected.media.ID)
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func streamChoices(media model.Media, streams []model.Stream, streamErr error) ([]navigationChoice, error) {
	if streamErr != nil {
		return nil, streamErr
	}
	if len(streams) == 0 {
		return nil, fmt.Errorf("no playable streams found")
	}
	result := make([]navigationChoice, len(streams))
	for i, stream := range streams {
		result[i] = navigationChoice{kind: navigationStream, media: media, stream: stream}
	}
	return result, nil
}
