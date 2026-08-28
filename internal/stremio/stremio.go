package stremio

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"lemmewatch/internal/model"
)

var (
	qualityRE = regexp.MustCompile(`(?i)\b(2160|1080|720|480)p\b`)
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
		InfoHash      string `json:"infoHash"`
		FileIdx       int    `json:"fileIdx"`
		URL           string `json:"url"`
		BehaviorHints struct {
			Filename string `json:"filename"`
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
	u.Path = path.Join(u.Path, "stream", mediaType, id+".json")
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
		if !validHash(hash) || raw.FileIdx < 0 || seen[hash+":"+strconv.Itoa(raw.FileIdx)] {
			continue
		}
		seen[hash+":"+strconv.Itoa(raw.FileIdx)] = true
		text := raw.Name + " " + raw.Title
		streams = append(streams, model.Stream{Hash: hash, FileIndex: raw.FileIdx, Title: displayTitle(raw.Title), Filename: raw.BehaviorHints.Filename, Quality: quality(text), Seeders: seeders(text), Size: size(text), Source: raw.Name})
	}
	Rank(streams)
	return streams, nil
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
		if a.Cached != b.Cached {
			return a.Cached
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
		return a.Hash < b.Hash
	})
}
