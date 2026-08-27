package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"path"
	"strconv"

	"lemmewatch/internal/model"
)

type Client struct {
	BaseURL string
	HTTP    *http.Client
}

type response struct {
	Metas []struct {
		ID          string `json:"id"`
		Type        string `json:"type"`
		Name        string `json:"name"`
		ReleaseInfo string `json:"releaseInfo"`
		Poster      string `json:"poster"`
		Description string `json:"description"`
	} `json:"metas"`
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
	for _, item := range payload.Metas {
		year, _ := strconv.Atoi(item.ReleaseInfo)
		items = append(items, model.Media{ID: item.ID, Type: model.MediaType(item.Type), Name: item.Name, Year: year, Poster: item.Poster, Summary: item.Description})
	}
	return items, nil
}
