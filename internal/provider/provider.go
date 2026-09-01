package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lemmewatch/internal/model"
	"lemmewatch/internal/stremio"
	"lemmewatch/internal/torbox"
)

const (
	TorBoxID     = "torbox"
	WebStreamrID = "webstreamr"
	PenguID      = "pengu"
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
	return directStreams(ctx, p.ID(), p.Client, request)
}

func (p WebStreamr) Resolve(ctx context.Context, stream model.Stream) (model.Playback, error) {
	return resolveDirect(ctx, p.ID(), "WebStreamr", p.Client, stream)
}

type Pengu struct {
	Client stremio.Client
}

func (Pengu) ID() string { return PenguID }

func (p Pengu) Streams(ctx context.Context, request Request) ([]model.Stream, error) {
	return directStreams(ctx, p.ID(), p.Client, request)
}

func (p Pengu) Resolve(ctx context.Context, stream model.Stream) (model.Playback, error) {
	return resolveDirect(ctx, p.ID(), "Pengu", p.Client, stream)
}

func directStreams(ctx context.Context, providerID string, client stremio.Client, request Request) ([]model.Stream, error) {
	streams, err := lookup(client, ctx, request)
	if err != nil {
		return nil, err
	}
	direct := streams[:0]
	for _, stream := range streams {
		if stream.URL == "" {
			continue
		}
		stream.Provider = providerID
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

func resolveDirect(ctx context.Context, providerID, providerName string, client stremio.Client, stream model.Stream) (model.Playback, error) {
	if stream.Provider != providerID || stream.URL == "" {
		return model.Playback{}, fmt.Errorf("invalid %s stream", providerName)
	}
	if len(stream.Headers) > 0 {
		return model.Playback{}, fmt.Errorf("stream requires unsupported request headers")
	}
	resolved, err := resolveHTTP(ctx, client.HTTP, stream.URL)
	if err != nil {
		return model.Playback{}, err
	}
	return model.Playback{URL: resolved}, nil
}

func resolveHTTP(ctx context.Context, httpClient *http.Client, rawURL string) (string, error) {
	if httpClient == nil {
		return rawURL, nil
	}
	client := *httpClient
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	current := rawURL
	for range 8 {
		if direct := unwrapDownloadURL(current); direct != "" {
			return direct, nil
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return "", fmt.Errorf("stream resolution failed")
		}
		req.Header.Set("Range", "bytes=0-0")
		req.Header.Set("User-Agent", "lemmewatch/0.1")
		res, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("stream resolution failed")
		}
		res.Body.Close()
		if res.StatusCode >= 300 && res.StatusCode < 400 {
			location, err := res.Location()
			if err != nil {
				return "", fmt.Errorf("stream resolution returned invalid redirect")
			}
			current = location.String()
			continue
		}
		contentType := strings.ToLower(res.Header.Get("Content-Type"))
		if res.StatusCode >= 200 && res.StatusCode < 300 && !strings.Contains(contentType, "text/html") {
			return current, nil
		}
		return "", fmt.Errorf("stream resolution returned no media")
	}
	return "", fmt.Errorf("stream resolution returned too many redirects")
}

func unwrapDownloadURL(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil || !strings.EqualFold(u.Hostname(), "gamerxyt.com") || u.Path != "/dl.php" {
		return ""
	}
	direct := u.Query().Get("link")
	parsed, err := url.Parse(direct)
	if err != nil || parsed.Host == "" || !(strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) {
		return ""
	}
	return parsed.String()
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
