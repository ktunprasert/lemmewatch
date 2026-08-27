package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"
	"time"

	"lemmewatch/internal/catalog"
	"lemmewatch/internal/config"
	"lemmewatch/internal/model"
	"lemmewatch/internal/player"
	"lemmewatch/internal/selector"
	"lemmewatch/internal/stremio"
	"lemmewatch/internal/torbox"
)

type App struct {
	Catalog catalog.Client
	Streams stremio.Client
	TorBox  torbox.Client
	Player  player.Player
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
}

type navigationKind int

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
		if n.media.Year > 0 {
			return fmt.Sprintf("%s (%d)", n.media.Name, n.media.Year)
		}
		return fmt.Sprintf("%s (%s)", n.media.Name, n.media.ID)
	case navigationSeason:
		return fmt.Sprintf("Season %d", n.season)
	case navigationEpisode:
		return fmt.Sprintf("Episode %d  %s", n.episode.Episode, n.episode.Title)
	case navigationStream:
		return streamLabel(n.stream)
	default:
		return "Unknown"
	}
}

func (n navigationChoice) Group() string {
	if n.kind == navigationMedia {
		return string(n.media.Type)
	}
	return ""
}
func (n navigationChoice) Terminal() bool { return n.kind == navigationStream }
func (n navigationChoice) StreamInfo() (bool, int, bool) {
	return n.stream.Cached, n.stream.Quality, n.kind == navigationStream
}

func streamLabel(s model.Stream) string {
	cache := "uncached"
	if s.Cached {
		cache = "cached"
	}
	quality := "unknown"
	if s.Quality > 0 {
		quality = fmt.Sprintf("%dp", s.Quality)
	}
	return fmt.Sprintf("%s  [%s]  %s", quality, cache, s.Title)
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
	fmt.Fprintln(a.Err, "Querying stream addon...")
	return a.Streams.Streams(ctx, imdbID)
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
	choices := make([]navigationChoice, len(items))
	for i, item := range items {
		choices[i] = navigationChoice{kind: navigationMedia, media: item}
	}
	preferences := config.Load()
	_, err = selector.Browse(ctx, a.In, a.Out, choices, func(ctx context.Context, selected navigationChoice) ([]navigationChoice, error) {
		switch selected.kind {
		case navigationMedia:
			if selected.media.Type == model.Movie {
				streams, streamErr := a.Streams.Streams(ctx, selected.media.ID)
				return a.prepareStreams(ctx, selected.media, streams, streamErr)
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
			streams, streamErr := a.Streams.SeriesStreams(ctx, selected.episode.ID)
			for i := range streams {
				streams[i].Season = selected.episode.Season
				streams[i].Episode = selected.episode.Episode
			}
			return a.prepareStreams(ctx, selected.media, streams, streamErr)
		default:
			return nil, fmt.Errorf("item cannot be opened")
		}
	}, selector.BrowserOptions[navigationChoice]{
		InitialQuery:     query,
		ParentGroups:     []string{string(model.Movie), string(model.Series)},
		PreferredGroup:   preferences.MediaTab,
		PreferredQuality: preferences.Quality,
		ChildTitle: func(selected navigationChoice) string {
			switch selected.kind {
			case navigationMedia:
				if selected.media.Type == model.Series {
					return "Seasons"
				}
				return "Torrents"
			case navigationSeason:
				return "Episodes"
			default:
				return "Torrents"
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
		Play: func(playContext context.Context, selected navigationChoice) error {
			resolved, err := a.TorBox.ResolveFile(playContext, selected.stream.Hash, selected.stream.FileIndex, selected.stream.Filename, selected.stream.Season, selected.stream.Episode)
			if err != nil {
				return err
			}
			if err := config.RecordHistory(config.HistoryEntry{ID: selected.media.ID, Title: selected.media.Name, Type: string(selected.media.Type)}); err != nil {
				return fmt.Errorf("record history: %w", err)
			}
			if err := a.Player.Play(playContext, resolved); err != nil {
				if playContext.Err() != nil {
					return playContext.Err()
				}
				return err
			}
			return nil
		},
		Requery: func(searchContext context.Context, query string) ([]navigationChoice, error) {
			results, err := a.searchCatalog(searchContext, query, "")
			if err != nil {
				return nil, err
			}
			choices := make([]navigationChoice, len(results))
			for i, result := range results {
				choices[i] = navigationChoice{kind: navigationMedia, media: result}
			}
			return choices, nil
		},
	})
	if err != nil {
		return err
	}
	return nil
}

func (a App) prepareStreams(ctx context.Context, media model.Media, streams []model.Stream, streamErr error) ([]navigationChoice, error) {
	if streamErr != nil {
		return nil, streamErr
	}
	if a.TorBox.Token == "" {
		return nil, fmt.Errorf("TORBOX_API_TOKEN is required")
	}
	if len(streams) == 0 {
		return nil, fmt.Errorf("no playable streams found")
	}
	hashes := make([]string, len(streams))
	for i := range streams {
		hashes[i] = streams[i].Hash
	}
	cacheCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cached, err := a.TorBox.Cached(cacheCtx, hashes)
	if err != nil {
		return nil, err
	}
	for i := range streams {
		streams[i].Cached = cached[streams[i].Hash]
	}
	stremio.Rank(streams)
	result := make([]navigationChoice, len(streams))
	for i, stream := range streams {
		result[i] = navigationChoice{kind: navigationStream, media: media, stream: stream}
	}
	return result, nil
}
