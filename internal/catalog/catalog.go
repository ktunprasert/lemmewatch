package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
	"time"

	"lemmewatch/internal/metadata"
	"lemmewatch/internal/model"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type response struct {
	Metas []json.RawMessage `json:"metas"`
}

type mediaResponse struct {
	ID          string `json:"id"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	ReleaseInfo string `json:"releaseInfo"`
	Poster      string `json:"poster"`
	Description string `json:"description"`
	Rating      string `json:"imdbRating"`
}

type metaResponse struct {
	Meta json.RawMessage `json:"meta"`
}

type videoResponse struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Title    string `json:"title"`
	Season   int    `json:"season"`
	Episode  int    `json:"episode"`
	Released string `json:"released"`
	Rating   string `json:"rating"`
}

func (c Client) Search(ctx context.Context, kind model.MediaType, query string) ([]model.Media, error) {
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("catalog URL: %w", err)
	}
	u.Path = path.Join(u.Path, "catalog", string(kind), "top", "search="+query+".json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("catalog search: %w", err)
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return nil, fmt.Errorf("catalog search: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("catalog search: HTTP %d", res.StatusCode)
	}
	var payload response
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("catalog response: %w", err)
	}
	items := make([]model.Media, 0, len(payload.Metas))
	for _, raw := range payload.Metas {
		item, err := decodeMedia(raw)
		if err != nil {
			return nil, fmt.Errorf("catalog metadata: %w", err)
		}
		items = append(items, item)
	}
	return items, nil
}

func (c Client) Episodes(ctx context.Context, imdbID string) ([]model.Episode, error) {
	details, err := c.Details(ctx, model.Series, imdbID)
	return details.Episodes, err
}

func decodeMedia(raw json.RawMessage) (model.Media, error) {
	var item mediaResponse
	if err := json.Unmarshal(raw, &item); err != nil {
		return model.Media{}, err
	}
	yearText := item.ReleaseInfo
	if len(yearText) >= 4 {
		yearText = yearText[:4]
	}
	year, _ := strconv.Atoi(yearText)
	return model.Media{ID: item.ID, Type: model.MediaType(item.Type), Name: item.Name, Year: year, Poster: item.Poster, Summary: item.Description, Rating: item.Rating, Metadata: metadata.Parse(raw, "videos")}, nil
}

func (c Client) Details(ctx context.Context, kind model.MediaType, imdbID string) (model.MediaDetails, error) {
	var details model.MediaDetails
	if kind != model.Movie && kind != model.Series {
		return details, fmt.Errorf("unsupported metadata type")
	}
	u, err := url.Parse(c.BaseURL)
	if err != nil {
		return details, fmt.Errorf("catalog URL: %w", err)
	}
	u.Path = path.Join(u.Path, "meta", string(kind), imdbID+".json")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return details, fmt.Errorf("title metadata request failed")
	}
	res, err := c.HTTP.Do(req)
	if err != nil {
		return details, fmt.Errorf("title metadata request failed")
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return details, fmt.Errorf("title metadata: HTTP %d", res.StatusCode)
	}
	var payload metaResponse
	if err := json.NewDecoder(res.Body).Decode(&payload); err != nil {
		return details, fmt.Errorf("invalid title metadata response")
	}
	if len(payload.Meta) == 0 || strings.TrimSpace(string(payload.Meta)) == "null" {
		return details, fmt.Errorf("title metadata missing")
	}
	details.Media, err = decodeMedia(payload.Meta)
	if err != nil {
		return details, fmt.Errorf("invalid title metadata")
	}
	if details.Media.ID == "" {
		details.Media.ID = imdbID
	}
	if details.Media.Type == "" {
		details.Media.Type = kind
	}
	var children struct {
		Videos []json.RawMessage `json:"videos"`
	}
	if json.Unmarshal(payload.Meta, &children) != nil {
		return details, fmt.Errorf("invalid episode metadata")
	}
	for _, raw := range children.Videos {
		var video videoResponse
		if json.Unmarshal(raw, &video) != nil {
			return details, fmt.Errorf("invalid episode metadata")
		}
		if video.Season <= 0 || video.Episode <= 0 {
			continue
		}
		title := video.Name
		if title == "" {
			title = video.Title
		}
		released, _ := time.Parse(time.RFC3339, video.Released)
		rating := video.Rating
		if rating == "0" {
			rating = ""
		}
		details.Episodes = append(details.Episodes, model.Episode{ID: video.ID, Title: title, Season: video.Season, Episode: video.Episode, Released: released, Rating: rating, Metadata: metadata.Parse(raw)})
	}
	return details, nil
}
