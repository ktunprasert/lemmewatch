package app

import (
	"fmt"
	"strings"
	"time"

	"lemmewatch/internal/model"
)

// InfoLines exposes available metadata without fetching additional data.
func (n navigationChoice) InfoLines(watched map[string]bool) []string {
	switch n.kind {
	case navigationMedia:
		kind := ""
		switch n.media.Type {
		case model.Movie:
			kind = "Movie"
		case model.Series:
			kind = "Series"
		}
		year := ""
		if n.media.Year > 0 {
			year = fmt.Sprint(n.media.Year)
		}
		lines := []string{n.media.Name, joinInfo(kind, year, infoRating(n.media.Rating))}
		playedAt := n.playedAt
		if playedAt.IsZero() {
			playedAt = n.media.PlayedAt
		}
		if !playedAt.IsZero() {
			lines = append(lines, "Last played: "+playedAt.Local().Format("2006-01-02 15:04"))
		}
		return append(lines, n.media.Summary)
	case navigationSeason:
		lines := []string{fmt.Sprintf("Season %d · %d episodes", n.season, len(n.episodes))}
		var first, last time.Time
		seen := 0
		for _, episode := range n.episodes {
			if watched[fmt.Sprintf("%s:%d:%d", n.media.ID, episode.Season, episode.Episode)] {
				seen++
			}
			if episode.Released.IsZero() {
				continue
			}
			if first.IsZero() || episode.Released.Before(first) {
				first = episode.Released
			}
			if last.IsZero() || episode.Released.After(last) {
				last = episode.Released
			}
		}
		if n.media.ID != "" && len(n.episodes) > 0 {
			lines = append(lines, fmt.Sprintf("Watched: %d/%d", seen, len(n.episodes)))
		}
		if !first.IsZero() {
			released := first.Format("2006-01-02")
			if end := last.Format("2006-01-02"); end != released {
				released += " – " + end
			}
			lines = append(lines, "Released: "+released)
		}
		return append(lines, n.media.Name)
	case navigationEpisode:
		status := ""
		if n.media.ID != "" {
			status = "Unwatched"
			if watched[fmt.Sprintf("%s:%d:%d", n.media.ID, n.episode.Season, n.episode.Episode)] {
				status = "Watched"
			}
		}
		released := ""
		if !n.episode.Released.IsZero() {
			released = "Released: " + n.episode.Released.Format("2006-01-02")
		}
		return []string{
			n.episode.Title,
			joinInfo(fmt.Sprintf("S%02dE%02d", n.episode.Season, n.episode.Episode), status),
			joinInfo(released, infoRating(n.episode.Rating)),
			n.media.Name,
		}
	case navigationStream:
		quality, seeders := "", ""
		if n.stream.Quality > 0 {
			quality = fmt.Sprintf("%dp", n.stream.Quality)
		}
		if n.stream.Seeders > 0 || n.stream.Cache == model.CacheCached || n.stream.Cache == model.CacheUncached {
			seeders = fmt.Sprintf("%d seeders", n.stream.Seeders)
		}
		availability := ""
		switch n.stream.Cache {
		case model.CacheCached:
			availability = "Cached"
		case model.CacheUncached:
			availability = "Uncached"
		case model.CacheNotApplicable:
			availability = "Direct"
		}
		if !n.stream.Playable && n.stream.Cache == model.CacheNotApplicable {
			availability = joinInfo(availability, "Not playable")
		}
		lines := []string{
			n.stream.Title,
			joinInfo(quality, formatSize(n.stream.Size), seeders),
			joinInfo(availability, n.stream.Provider, n.stream.Source),
		}
		if n.stream.Filename != "" && n.stream.Filename != n.stream.Title {
			lines = append(lines, "File: "+n.stream.Filename)
		}
		if len(n.stream.AudioLanguages) > 0 {
			lines = append(lines, "Audio (release): "+strings.Join(n.stream.AudioLanguages, ", "))
		}
		if len(n.stream.SubtitleLanguages) > 0 {
			lines = append(lines, "Subtitles (release): "+strings.Join(n.stream.SubtitleLanguages, ", "))
		}
		if len(n.stream.LanguageHints) > 0 {
			lines = append(lines, "Language hints: "+strings.Join(n.stream.LanguageHints, ", "))
		}
		return lines
	default:
		return []string{n.Label()}
	}
}

func infoRating(rating string) string {
	if rating == "" {
		return ""
	}
	return "Rating " + rating
}

func joinInfo(values ...string) string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			parts = append(parts, value)
		}
	}
	return strings.Join(parts, " · ")
}
