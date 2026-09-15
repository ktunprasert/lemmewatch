package app

import (
	"context"
	"fmt"

	"lemmewatch/internal/model"
	"lemmewatch/internal/storage"
)

func (a App) catalogDetails(ctx context.Context, kind model.MediaType, id string, refresh bool) (model.MediaDetails, error) {
	key := storage.SourceFingerprint(a.Catalog.BaseURL) + ":" + string(kind) + ":" + id
	var details model.MediaDetails
	if !refresh && a.Storage != nil {
		if hit, _ := a.Storage.CacheGet(storage.CacheMetadata, key, &details); hit {
			return details, nil
		}
	}
	details, err := a.Catalog.Details(ctx, kind, id)
	if err == nil && a.Storage != nil {
		_ = a.Storage.CachePut(storage.CacheMetadata, key, details, seriesCacheTTL)
		if kind == model.Series {
			_ = a.Storage.CachePut(storage.CacheSeries, a.seriesCacheKey(id), details.Episodes, seriesCacheTTL)
		}
	}
	return details, err
}

func mergeMediaDetails(current, full model.Media) model.Media {
	if current.Name == "" && full.Name != "" {
		current.Name = full.Name
	}
	if full.Year != 0 {
		current.Year = full.Year
	}
	if full.Summary != "" {
		current.Summary = full.Summary
	}
	if full.Rating != "" {
		current.Rating = full.Rating
	}
	if full.Poster != "" {
		current.Poster = full.Poster
	}
	current.Metadata = full.Metadata
	return current
}

func (n navigationChoice) InfoKey() string {
	switch n.kind {
	case navigationMedia:
		return "media:" + string(n.media.Type) + ":" + n.media.ID
	case navigationSeason:
		return fmt.Sprintf("season:%s:%d", n.media.ID, n.season)
	case navigationEpisode:
		return fmt.Sprintf("episode:%s:%d:%d", n.media.ID, n.episode.Season, n.episode.Episode)
	case navigationStream:
		identity := n.stream.Hash
		if identity == "" {
			identity = storage.SourceFingerprint(n.stream.URL)
		}
		return fmt.Sprintf("stream:%s:%s:%d:%s:%d:%d:%s", n.stream.Provider, identity, n.stream.FileIndex, n.media.ID, n.episode.Season, n.episode.Episode, storage.SourceFingerprint(n.stream.Filename))
	default:
		return ""
	}
}

func (n navigationChoice) ModeNeedsInfo(mode string) bool {
	if mode != "r" {
		return false
	}
	if n.kind == navigationMedia {
		return ratingLabel(n.media.Rating) == "--"
	}
	return n.kind == navigationEpisode && ratingLabel(n.episode.Rating) == "--" && ratingLabel(n.media.Rating) == "--"
}

func ratingLabel(rating string) string {
	if rating == "" || rating == "0" || rating == "0.0" || rating == "N/A" {
		return "--"
	}
	return rating
}

// WithInfo overlays provider details on current navigation state. History times,
// availability and other local fields must not come from an older info snapshot.
func (n navigationChoice) WithInfo(full navigationChoice) navigationChoice {
	n.media = mergeMediaDetails(n.media, full.media)
	if n.kind == navigationEpisode {
		n.episode = full.episode
	}
	if n.kind == navigationSeason {
		n.episodes = full.episodes
	}
	if n.kind == navigationStream {
		n.stream.Metadata = full.stream.Metadata
		n.stream.CacheMetadata = full.stream.CacheMetadata
		n.stream.TorrentMetadata = full.stream.TorrentMetadata
	}
	return n
}

func (a App) navigationInfo(ctx context.Context, selected navigationChoice) (navigationChoice, error) {
	if selected.kind == navigationStream {
		p, err := a.provider(selected.stream.Provider)
		if err != nil {
			return selected, err
		}
		if loader, ok := p.(interface {
			Info(context.Context, model.Stream) (model.Stream, error)
		}); ok {
			selected.stream, err = loader.Info(ctx, selected.stream)
		}
		return selected, err
	}
	details, err := a.catalogDetails(ctx, selected.media.Type, selected.media.ID, false)
	if err != nil {
		return selected, err
	}
	selected.media = mergeMediaDetails(selected.media, details.Media)
	if selected.kind == navigationSeason {
		selected.episodes = nil
		for _, episode := range details.Episodes {
			if episode.Season == selected.season {
				selected.episodes = append(selected.episodes, episode)
			}
		}
	}
	if selected.kind == navigationEpisode {
		for _, episode := range details.Episodes {
			if episode.Season == selected.episode.Season && episode.Episode == selected.episode.Episode {
				selected.episode = episode
				break
			}
		}
	}
	return selected, nil
}
