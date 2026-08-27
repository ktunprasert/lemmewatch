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

type mediaChoice struct{ model.Media }

func (m mediaChoice) Label() string {
	if m.Year > 0 {
		return fmt.Sprintf("%s (%d)", m.Name, m.Year)
	}
	return m.Name
}
func (m mediaChoice) Group() string { return string(m.Type) }

type streamChoice struct{ model.Stream }

func (s streamChoice) Label() string {
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
func (s streamChoice) IsCached() bool    { return s.Cached }
func (s streamChoice) VideoQuality() int { return s.Quality }

func (a App) Search(ctx context.Context, query string, kind model.MediaType) ([]model.Media, error) {
	fmt.Fprintf(a.Err, "Searching catalog for %q...\n", query)
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
	sort.SliceStable(items, func(i, j int) bool { return items[i].Name < items[j].Name })
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
	choices := make([]mediaChoice, len(items))
	for i, item := range items {
		choices[i] = mediaChoice{item}
	}
	preferences := config.Load()
	chosen, err := selector.Browse(ctx, a.In, a.Out, choices, func(ctx context.Context, media mediaChoice) ([]streamChoice, error) {
		if media.Type == model.Series {
			return nil, fmt.Errorf("series season and episode selection is not implemented yet")
		}
		if a.TorBox.Token == "" {
			return nil, fmt.Errorf("TORBOX_API_TOKEN is required")
		}
		streams, err := a.Streams.Streams(ctx, media.ID)
		if err != nil {
			return nil, err
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
			stream := &streams[i]
			stream.Cached = cached[stream.Hash]
		}
		stremio.Rank(streams)
		result := make([]streamChoice, len(streams))
		for i, stream := range streams {
			result[i] = streamChoice{stream}
		}
		return result, nil
	}, selector.BrowserOptions{
		ParentGroups:     []string{string(model.Movie), string(model.Series)},
		PreferredQuality: preferences.Quality,
		SaveQuality: func(quality int) error {
			preferences.Quality = quality
			return config.Save(preferences)
		},
	})
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Err, "Resolving stream through TorBox...")
	resolved, err := a.TorBox.Resolve(ctx, chosen.Hash, chosen.FileIndex)
	if err != nil {
		return err
	}
	fmt.Fprintln(a.Err, "Launching player...")
	return a.Player.Play(ctx, resolved)
}
