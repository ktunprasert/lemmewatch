package app

import (
	"context"
	"fmt"
	"io"
	"sort"
	"sync"

	"lemmewatch/internal/catalog"
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

type streamChoice struct{ model.Stream }

func (s streamChoice) Label() string {
	cache := ""
	if s.Cached {
		cache = " [cached]"
	}
	quality := "unknown"
	if s.Quality > 0 {
		quality = fmt.Sprintf("%dp", s.Quality)
	}
	return fmt.Sprintf("%s%s  seeds:%d  %.2f GB  %s", quality, cache, s.Seeders, float64(s.Size)/1e9, s.Title)
}

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
	fmt.Fprintln(a.Err, "Checking TorBox cache...")
	return a.TorBox.Cached(ctx, hashes)
}

func (a App) Watch(ctx context.Context, query string) error {
	items, err := a.Search(ctx, query, model.Movie)
	if err != nil {
		return err
	}
	choices := make([]mediaChoice, len(items))
	for i, item := range items {
		choices[i] = mediaChoice{item}
	}
	movie, err := selector.Choose(ctx, a.In, a.Out, choices)
	if err != nil {
		return err
	}
	streams, err := a.LookupStreams(ctx, movie.ID)
	if err != nil {
		return err
	}
	if len(streams) == 0 {
		return fmt.Errorf("no playable streams found")
	}
	hashes := make([]string, len(streams))
	for i := range streams {
		hashes[i] = streams[i].Hash
	}
	cached, err := a.Cache(ctx, hashes)
	if err != nil {
		return err
	}
	available := streams[:0]
	for _, stream := range streams {
		stream.Cached = cached[stream.Hash]
		if stream.Cached {
			available = append(available, stream)
		}
	}
	if len(available) == 0 {
		return fmt.Errorf("no cached streams found")
	}
	stremio.Rank(available)
	streamChoices := make([]streamChoice, len(available))
	for i, stream := range available {
		streamChoices[i] = streamChoice{stream}
	}
	chosen, err := selector.Choose(ctx, a.In, a.Out, streamChoices)
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
