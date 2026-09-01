package stremio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"lemmewatch/internal/model"
)

var (
	qualityRE = regexp.MustCompile(`(?i)\b(2160|1080|720|480|360)p\b`)
	seedersRE = regexp.MustCompile(`(?i)(?:👤|seed(?:ers?)?\s*[:=]?)\s*(\d+)`)
	sizeRE    = regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*(GB|MB)\b`)
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type response struct {
	Streams []struct {
		Name          string `json:"name"`
		Title         string `json:"title"`
		Description   string `json:"description"`
		InfoHash      string `json:"infoHash"`
		FileIdx       int    `json:"fileIdx"`
		URL           string `json:"url"`
		BehaviorHints struct {
			Filename     string `json:"filename"`
			NotWebReady  bool   `json:"notWebReady"`
			VideoSize    int64  `json:"videoSize"`
			ProxyHeaders struct {
				Request map[string]string `json:"request"`
			} `json:"proxyHeaders"`
		} `json:"behaviorHints"`
	} `json:"streams"`
}

func (c Client) Streams(ctx context.Context, imdbID string) ([]model.Stream, error) {
	return c.streams(ctx, "movie", imdbID)
}

func (c Client) SeriesStreams(ctx context.Context, episodeID string) ([]model.Stream, error) {
	return c.streams(ctx, "series", episodeID)
}

func (c Client) streams(ctx context.Context, mediaType, id string) ([]model.Stream, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("stream addon URL: %w", err)
	}
	basePath := strings.TrimSuffix(u.EscapedPath(), "/")
	if strings.HasSuffix(basePath, "/manifest.json") {
		basePath = strings.TrimSuffix(basePath, "/manifest.json")
	}
	escapedPath := basePath + "/stream/" + url.PathEscape(mediaType) + "/" + url.PathEscape(id) + ".json"
	u.RawPath = escapedPath
	u.Path, err = url.PathUnescape(escapedPath)
	if err != nil {
		return nil, fmt.Errorf("stream addon URL: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("stream lookup: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lemmewatch/0.1")
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("stream lookup: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("stream lookup: HTTP %d", res.StatusCode)
	}
	var payload response
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("stream response: %w", err)
	}
	seen := make(map[string]bool)
	streams := make([]model.Stream, 0, len(payload.Streams))
	for _, raw := range payload.Streams {
		hash := strings.ToLower(raw.InfoHash)
		if hash == "" && strings.HasPrefix(raw.URL, "magnet:") {
			if u, err := url.Parse(raw.URL); err == nil {
				for _, xt := range u.Query()["xt"] {
					if strings.HasPrefix(strings.ToLower(xt), "urn:btih:") {
						hash = strings.ToLower(strings.TrimPrefix(xt, "urn:btih:"))
						break
					}
				}
			}
		}
		if !validHash(hash) {
			hash = ""
		}
		streamURL := ""
		if raw.URL != "" && !strings.HasPrefix(strings.ToLower(raw.URL), "magnet:") {
			if parsed, err := url.Parse(raw.URL); err == nil && (strings.EqualFold(parsed.Scheme, "http") || strings.EqualFold(parsed.Scheme, "https")) && parsed.Host != "" {
				streamURL = parsed.String()
			}
		}
		key := ""
		if hash != "" && raw.FileIdx >= 0 {
			key = "torrent:" + hash + ":" + strconv.Itoa(raw.FileIdx)
		} else if streamURL != "" {
			key = "url:" + streamURL
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		text := raw.Name + " " + raw.Title + " " + raw.Description
		title := streamTitle(raw.Title, raw.Description, raw.Name)
		streamSize := size(text)
		if streamSize == 0 {
			streamSize = raw.BehaviorHints.VideoSize
		}
		streams = append(streams, model.Stream{Hash: hash, URL: streamURL, Headers: raw.BehaviorHints.ProxyHeaders.Request, FileIndex: raw.FileIdx, Title: title, Filename: raw.BehaviorHints.Filename, Quality: quality(text), Seeders: seeders(text), Size: streamSize, NotWebReady: raw.BehaviorHints.NotWebReady, Source: raw.Name})
	}
	return streams, nil
}

func streamTitle(title, description, name string) string {
	if title = displayTitle(title); title != "" {
		return title
	}
	lines := make([]string, 0, 2)
	for line := range strings.SplitSeq(description, "\n") {
		if line = displayTitle(line); line != "" {
			lines = append(lines, line)
		}
	}
	if len(lines) > 1 {
		return lines[1]
	}
	if len(lines) == 1 {
		return lines[0]
	}
	return displayTitle(name)
}

func displayTitle(value string) string {
	for line := range strings.SplitSeq(value, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		line = strings.Map(func(r rune) rune {
			if unicode.Is(unicode.So, r) || r == '\ufe0f' || r == '\u200d' {
				return -1
			}
			return r
		}, line)
		return strings.Join(strings.Fields(line), " ")
	}
	return ""
}

func validHash(hash string) bool { b, err := hex.DecodeString(hash); return err == nil && len(b) == 20 }
func quality(s string) int {
	if m := qualityRE.FindStringSubmatch(s); len(m) > 0 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}
func seeders(s string) int {
	if m := seedersRE.FindStringSubmatch(s); len(m) > 0 {
		n, _ := strconv.Atoi(m[1])
		return n
	}
	return 0
}
func size(s string) int64 {
	if m := sizeRE.FindStringSubmatch(s); len(m) > 0 {
		n, _ := strconv.ParseFloat(m[1], 64)
		unit := 1e6
		if strings.EqualFold(m[2], "GB") {
			unit = 1e9
		}
		return int64(n * unit)
	}
	return 0
}

func Rank(streams []model.Stream) {
	sort.SliceStable(streams, func(i, j int) bool {
		a, b := streams[i], streams[j]
		if a.Cache != b.Cache {
			return a.Cache == model.CacheCached
		}
		if a.Quality != b.Quality {
			return a.Quality > b.Quality
		}
		if a.Seeders != b.Seeders {
			return a.Seeders > b.Seeders
		}
		if a.Size != b.Size {
			return a.Size < b.Size
		}
		if a.Hash != b.Hash {
			return a.Hash < b.Hash
		}
		return a.URL < b.URL
	})
}
