package torbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

type envelope[T any] struct {
	Success bool   `json:"success"`
	Detail  string `json:"detail"`
	Data    T      `json:"data"`
}

type torrent struct {
	ID    int64  `json:"id"`
	Files []file `json:"files"`
}

type file struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"short_name"`
	Size      int64  `json:"size"`
}

func (c Client) Cached(ctx context.Context, hashes []string) (map[string]bool, error) {
	unique := make([]string, 0, len(hashes))
	seen := make(map[string]bool, len(hashes))
	for _, hash := range hashes {
		if !validHash(hash) {
			return nil, fmt.Errorf("invalid torrent info hash")
		}
		if !seen[hash] {
			seen[hash] = true
			unique = append(unique, hash)
		}
	}
	u, err := c.endpoint("torrents/checkcached")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("format", "object")
	u.RawQuery = q.Encode()
	result := make(map[string]bool, len(unique))
	const batchSize = 500
	for start := 0; start < len(unique); start += batchSize {
		end := min(start+batchSize, len(unique))
		body, err := json.Marshal(map[string][]string{"hashes": unique[start:end]})
		if err != nil {
			return nil, fmt.Errorf("TorBox cache request: %w", err)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("TorBox cache request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		c.authorize(req)
		res, err := c.HTTP.Do(req)
		if err != nil {
			return nil, fmt.Errorf("TorBox cache check: %w", requestError("request", err))
		}
		var payload envelope[map[string]json.RawMessage]
		decodeErr := json.NewDecoder(res.Body).Decode(&payload)
		res.Body.Close()
		if res.StatusCode < 200 || res.StatusCode >= 300 {
			return nil, fmt.Errorf("TorBox cache check: HTTP %d", res.StatusCode)
		}
		if decodeErr != nil || !payload.Success {
			return nil, fmt.Errorf("TorBox cache check returned invalid response")
		}
		for hash, raw := range payload.Data {
			var cached bool
			if json.Unmarshal(raw, &cached) == nil {
				result[hash] = cached
			} else {
				result[hash] = string(raw) != "null" && string(raw) != "false"
			}
		}
	}
	return result, nil
}

func (c Client) Resolve(ctx context.Context, hash string, videoIndex int) (string, error) {
	return c.ResolveFile(ctx, hash, videoIndex, "", 0, 0)
}

func (c Client) ResolveFile(ctx context.Context, hash string, videoIndex int, filename string, season, episode int) (string, error) {
	if !validHash(hash) {
		return "", fmt.Errorf("invalid torrent info hash")
	}
	if videoIndex < 0 {
		return "", fmt.Errorf("video file index must not be negative")
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	_ = form.WriteField("magnet", "magnet:?xt=urn:btih:"+hash)
	if err := form.Close(); err != nil {
		return "", fmt.Errorf("TorBox torrent request: %w", err)
	}
	u, err := c.endpoint("torrents/createtorrent")
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), &body)
	if err != nil {
		return "", fmt.Errorf("TorBox torrent request: %w", err)
	}
	req.Header.Set("Content-Type", form.FormDataContentType())
	c.authorize(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return "", requestError("TorBox torrent creation", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return "", fmt.Errorf("TorBox torrent creation: HTTP %d", res.StatusCode)
	}
	var created envelope[struct {
		TorrentID int64 `json:"torrent_id"`
	}]
	if json.NewDecoder(res.Body).Decode(&created) != nil || !created.Success {
		return "", fmt.Errorf("TorBox torrent creation returned invalid response")
	}

	listURL, _ := c.endpoint("torrents/mylist")
	q := listURL.Query()
	q.Set("id", strconv.FormatInt(created.Data.TorrentID, 10))
	listURL.RawQuery = q.Encode()
	var listed envelope[torrent]
	if err := c.get(ctx, listURL, &listed); err != nil {
		return "", fmt.Errorf("TorBox file lookup: %w", err)
	}
	videos := listed.Data.videoFiles()
	selected, err := selectVideoFile(videos, videoIndex, filename, season, episode)
	if err != nil {
		return "", err
	}

	dlURL, _ := c.endpoint("torrents/requestdl")
	q = dlURL.Query()
	q.Set("token", c.Token)
	q.Set("torrent_id", strconv.FormatInt(created.Data.TorrentID, 10))
	q.Set("file_id", strconv.FormatInt(selected.ID, 10))
	q.Set("append_name", "true")
	dlURL.RawQuery = q.Encode()
	var download envelope[string]
	if err := c.get(ctx, dlURL, &download); err != nil {
		return "", fmt.Errorf("TorBox download resolution failed")
	}
	if download.Data == "" {
		return "", fmt.Errorf("TorBox download resolution returned no URL")
	}
	return download.Data, nil
}

func selectVideoFile(videos []file, videoIndex int, filename string, season, episode int) (file, error) {
	if filename != "" {
		for _, candidate := range videos {
			if strings.EqualFold(candidate.ShortName, filename) || strings.EqualFold(path.Base(candidate.Name), filename) {
				return candidate, nil
			}
		}
	}
	if season > 0 && episode > 0 {
		patterns := []*regexp.Regexp{
			regexp.MustCompile(fmt.Sprintf(`(?i)(?:^|[^a-z0-9])s0*%de0*%d(?:[^0-9]|$)`, season, episode)),
			regexp.MustCompile(fmt.Sprintf(`(?i)(?:^|[^0-9])0*%dx0*%d(?:[^0-9]|$)`, season, episode)),
		}
		var matches []file
		for _, candidate := range videos {
			for _, pattern := range patterns {
				if pattern.MatchString(candidate.Name) {
					matches = append(matches, candidate)
					break
				}
			}
		}
		if len(matches) > 0 {
			sort.SliceStable(matches, func(i, j int) bool { return matches[i].Size > matches[j].Size })
			return matches[0], nil
		}
	}
	if videoIndex < 0 || videoIndex >= len(videos) {
		return file{}, fmt.Errorf("TorBox video file index %d unavailable", videoIndex)
	}
	return videos[videoIndex], nil
}

func (t torrent) videoFiles() []file {
	var out []file
	for _, f := range t.Files {
		name := strings.ToLower(f.Name)
		for _, ext := range []string{".mkv", ".mp4", ".avi", ".mov", ".webm", ".m4v"} {
			if strings.HasSuffix(name, ext) {
				out = append(out, f)
				break
			}
		}
	}
	return out
}

func (c Client) endpoint(suffix string) (*url.URL, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("TorBox URL: %w", err)
	}
	u.Path = path.Join(u.Path, suffix)
	return u, nil
}
func (c Client) authorize(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.Token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "lemmewatch/0.1")
}
func (c Client) get(ctx context.Context, u *url.URL, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("request creation failed")
	}
	c.authorize(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return requestError("request", err)
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", res.StatusCode)
	}
	if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
		return fmt.Errorf("invalid response")
	}
	return nil
}

func requestError(operation string, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return fmt.Errorf("%s timed out", operation)
	case errors.Is(err, context.Canceled):
		return fmt.Errorf("%s cancelled", operation)
	default:
		return fmt.Errorf("%s connection failed", operation)
	}
}

func validHash(hash string) bool {
	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == 20
}
