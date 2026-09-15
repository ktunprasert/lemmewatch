package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"lemmewatch/internal/model"
	"lemmewatch/internal/storage"
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
	Title     string
	Season    int
	Episode   int
	Refresh   bool
}

type Provider interface {
	ID() string
	Streams(context.Context, Request) ([]model.Stream, error)
	Resolve(context.Context, model.Stream) (model.Playback, error)
}

type QueueProgress struct {
	TorrentID int64
	Progress  float64
}

type QueueTorrenter interface {
	QueueTorrent(ctx context.Context, stream model.Stream, notify func(QueueProgress)) error
}

type TorBox struct {
	StreamsClient stremio.Client
	TorBoxClient  torbox.Client
	Storage       *storage.Storage
}

func (TorBox) ID() string { return TorBoxID }

func (p TorBox) Streams(ctx context.Context, request Request) ([]model.Stream, error) {
	if p.TorBoxClient.Token == "" {
		return nil, fmt.Errorf("TORBOX_API_TOKEN is required")
	}
	streams, err := p.torrentCandidates(ctx, request)
	if err != nil {
		return nil, err
	}
	hashes := make([]string, len(streams))
	for i := range streams {
		hashes[i] = streams[i].Hash
	}
	cacheContext, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	cached, err := p.TorBoxClient.Cached(cacheContext, hashes)
	if err != nil {
		return nil, err
	}
	for i := range streams {
		streams[i].Provider = p.ID()
		streams[i].Season = request.Season
		streams[i].Episode = request.Episode
		streams[i].Playable = cached[streams[i].Hash]
		streams[i].Cache = model.CacheUncached
		if streams[i].Playable {
			streams[i].Cache = model.CacheCached
		}
	}
	stremio.Rank(streams, request.Title, request.Season, request.Episode)
	return streams, nil
}

const torrentCacheTTL = 24 * time.Hour

type torrentCandidate struct {
	Hash              string   `json:"hash"`
	FileIndex         int      `json:"file_index"`
	Title             string   `json:"title"`
	Filename          string   `json:"filename"`
	Quality           int      `json:"quality"`
	Seeders           int      `json:"seeders"`
	Size              int64    `json:"size"`
	NotWebReady       bool     `json:"not_web_ready"`
	Source            string   `json:"source"`
	AudioLanguages    []string `json:"audio_languages,omitempty"`
	SubtitleLanguages []string `json:"subtitle_languages,omitempty"`
	LanguageHints     []string `json:"language_hints,omitempty"`
}

func (p TorBox) torrentCandidates(ctx context.Context, request Request) ([]model.Stream, error) {
	key := "v2:" + storage.SourceFingerprint(p.StreamsClient.BaseURL) + ":" + string(request.MediaType) + ":" + request.ID
	var candidates []torrentCandidate
	if !request.Refresh && p.Storage != nil {
		if hit, _ := p.Storage.CacheGet(storage.CacheTorrents, key, &candidates); hit {
			return candidateStreams(candidates), nil
		}
	}

	streams, err := lookup(p.StreamsClient, ctx, request)
	if err != nil {
		return nil, err
	}
	for _, stream := range streams {
		if stream.Hash == "" {
			continue
		}
		candidates = append(candidates, torrentCandidate{
			Hash: stream.Hash, FileIndex: stream.FileIndex, Title: stream.Title,
			Filename: stream.Filename, Quality: stream.Quality, Seeders: stream.Seeders,
			Size: stream.Size, NotWebReady: stream.NotWebReady, Source: stream.Source,
			AudioLanguages: stream.AudioLanguages, SubtitleLanguages: stream.SubtitleLanguages, LanguageHints: stream.LanguageHints,
		})
	}
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no playable streams found")
	}
	if p.Storage != nil {
		_ = p.Storage.CachePut(storage.CacheTorrents, key, candidates, torrentCacheTTL)
	}
	return candidateStreams(candidates), nil
}

func candidateStreams(candidates []torrentCandidate) []model.Stream {
	streams := make([]model.Stream, len(candidates))
	for i, candidate := range candidates {
		streams[i] = model.Stream{
			Hash: candidate.Hash, FileIndex: candidate.FileIndex, Title: candidate.Title,
			Filename: candidate.Filename, Quality: candidate.Quality, Seeders: candidate.Seeders,
			Size: candidate.Size, NotWebReady: candidate.NotWebReady, Source: candidate.Source,
			AudioLanguages: candidate.AudioLanguages, SubtitleLanguages: candidate.SubtitleLanguages, LanguageHints: candidate.LanguageHints,
		}
	}
	return streams
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

func (p TorBox) QueueTorrent(ctx context.Context, stream model.Stream, notify func(QueueProgress)) error {
	if stream.Provider != p.ID() || stream.Hash == "" {
		return fmt.Errorf("invalid TorBox stream")
	}
	torrentID, err := p.TorBoxClient.Find(ctx, stream.Hash)
	if err != nil {
		return err
	}
	if torrentID == 0 {
		torrentID, err = p.TorBoxClient.Queue(ctx, stream.Hash)
		if err != nil {
			return err
		}
	}
	if notify != nil {
		notify(QueueProgress{TorrentID: torrentID})
	}
	return p.TorBoxClient.WaitDownloaded(ctx, torrentID, func(progress float64) {
		notify(QueueProgress{TorrentID: torrentID, Progress: progress})
	})
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
