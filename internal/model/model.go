package model

import "time"

type MediaType string

const (
	Movie  MediaType = "movie"
	Series MediaType = "series"
)

type Media struct {
	ID      string
	Type    MediaType
	Name    string
	Year    int
	Poster  string
	Summary string
	Rating  string
}

type Stream struct {
	Provider    string
	Hash        string
	URL         string
	Headers     map[string]string
	FileIndex   int
	Title       string
	Filename    string
	Quality     int
	Seeders     int
	Size        int64
	Cache       CacheStatus
	Playable    bool
	NotWebReady bool
	Source      string
	Season      int
	Episode     int
}

type CacheStatus string

const (
	CacheNotApplicable CacheStatus = "not-applicable"
	CacheCached        CacheStatus = "cached"
	CacheUncached      CacheStatus = "uncached"
)

type Playback struct {
	URL     string
	Headers map[string]string
}

type Episode struct {
	ID       string
	Title    string
	Season   int
	Episode  int
	Released time.Time
	Rating   string
}
