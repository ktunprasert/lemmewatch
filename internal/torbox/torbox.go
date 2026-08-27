package torbox

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/url"
	"path"
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
	for _, hash := range hashes {
		if !validHash(hash) {
			return nil, fmt.Errorf("invalid torrent info hash")
		}
	}
	u, err := c.endpoint("torrents/checkcached")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("hash", strings.Join(hashes, ","))
	q.Set("format", "object")
	u.RawQuery = q.Encode()
	var payload envelope[map[string]json.RawMessage]
	if err := c.get(ctx, u, &payload); err != nil {
		return nil, fmt.Errorf("TorBox cache check: %w", err)
	}
	result := make(map[string]bool, len(hashes))
	for _, hash := range hashes {
		raw, ok := payload.Data[hash]
		if !ok {
			continue
		}
		var cached bool
		if json.Unmarshal(raw, &cached) == nil {
			result[hash] = cached
		} else {
			result[hash] = string(raw) != "null" && string(raw) != "false"
		}
	}
	return result, nil
}

func (c Client) Resolve(ctx context.Context, hash string, videoIndex int) (string, error) {
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
		return "", fmt.Errorf("TorBox torrent creation failed")
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
	if videoIndex < 0 || videoIndex >= len(videos) {
		return "", fmt.Errorf("TorBox video file index %d unavailable", videoIndex)
	}

	dlURL, _ := c.endpoint("torrents/requestdl")
	q = dlURL.Query()
	q.Set("token", c.Token)
	q.Set("torrent_id", strconv.FormatInt(created.Data.TorrentID, 10))
	q.Set("file_id", strconv.FormatInt(videos[videoIndex].ID, 10))
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
func (c Client) authorize(req *http.Request) { req.Header.Set("Authorization", "Bearer "+c.Token) }
func (c Client) get(ctx context.Context, u *url.URL, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return fmt.Errorf("request creation failed")
	}
	c.authorize(req)
	res, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("request failed")
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

func validHash(hash string) bool {
	decoded, err := hex.DecodeString(hash)
	return err == nil && len(decoded) == 20
}
