package provider

import (
	"context"
	"fmt"
	"time"

	"lemmewatch/internal/model"
	"lemmewatch/internal/stremio"
	"lemmewatch/internal/torbox"
)

const (
	TorBoxID     = "torbox"
	WebStreamrID = "webstreamr"
)

type Request struct {
	MediaType model.MediaType
	ID        string
	Season    int
	Episode   int
}

type Provider interface {
	ID() string
	Streams(context.Context, Request) ([]model.Stream, error)
	Resolve(context.Context, model.Stream) (model.Playback, error)
}

type TorBox struct {
	StreamsClient stremio.Client
	TorBoxClient  torbox.Client
}

func (TorBox) ID() string { return TorBoxID }

func (p TorBox) Streams(ctx context.Context, request Request) ([]model.Stream, error) {
	if p.TorBoxClient.Token == "" {
		return nil, fmt.Errorf("TORBOX_API_TOKEN is required")
	}
	streams, err := lookup(p.StreamsClient, ctx, request)
	if err != nil {
		return nil, err
	}
	torrents := streams[:0]
	for _, stream := range streams {
		if stream.Hash != "" {
			torrents = append(torrents, stream)
		}
	}
	if len(torrents) == 0 {
		return nil, fmt.Errorf("no playable streams found")
	}
	hashes := make([]string, len(torrents))
	for i := range torrents {
		hashes[i] = torrents[i].Hash
	}
	cacheContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cached, err := p.TorBoxClient.Cached(cacheContext, hashes)
	if err != nil {
		return nil, err
	}
	for i := range torrents {
		torrents[i].Provider = p.ID()
		torrents[i].Season = request.Season
		torrents[i].Episode = request.Episode
		torrents[i].Playable = cached[torrents[i].Hash]
		torrents[i].Cache = model.CacheUncached
		if torrents[i].Playable {
			torrents[i].Cache = model.CacheCached
		}
	}
	stremio.Rank(torrents)
	return torrents, nil
}

func (p TorBox) Resolve(ctx context.Context, stream model.Stream) (model.Playback, error) {
	if stream.Provider != p.ID() || stream.Hash == "" {
		return model.Playback{}, fmt.Errorf("invalid TorBox stream")
	}
	resolved, err := p.TorBoxClient.ResolveFile(ctx, stream.Hash, stream.FileIndex, stream.Filename, stream.Season, stream.Episode)
	if err != nil {
		return model.Playback{}, err
	}
	return model.Playback{URL: resolved}, nil
}

type WebStreamr struct {
	Client stremio.Client
}

func (WebStreamr) ID() string { return WebStreamrID }

func (p WebStreamr) Streams(ctx context.Context, request Request) ([]model.Stream, error) {
	streams, err := lookup(p.Client, ctx, request)
	if err != nil {
		return nil, err
	}
	direct := streams[:0]
	for _, stream := range streams {
		if stream.URL == "" {
			continue
		}
		stream.Provider = p.ID()
		stream.Season = request.Season
		stream.Episode = request.Episode
		stream.Cache = model.CacheNotApplicable
		stream.Playable = len(stream.Headers) == 0
		direct = append(direct, stream)
	}
	if len(direct) == 0 {
		return nil, fmt.Errorf("no playable streams found")
	}
	return direct, nil
}

func (p WebStreamr) Resolve(_ context.Context, stream model.Stream) (model.Playback, error) {
	if stream.Provider != p.ID() || stream.URL == "" {
		return model.Playback{}, fmt.Errorf("invalid WebStreamr stream")
	}
	if len(stream.Headers) > 0 {
		return model.Playback{}, fmt.Errorf("stream requires unsupported request headers")
	}
	return model.Playback{URL: stream.URL}, nil
}

func lookup(client stremio.Client, ctx context.Context, request Request) ([]model.Stream, error) {
	switch request.MediaType {
	case model.Movie:
		return client.Streams(ctx, request.ID)
	case model.Series:
		return client.SeriesStreams(ctx, request.ID)
	default:
		return nil, fmt.Errorf("unsupported media type %q", request.MediaType)
	}
}
