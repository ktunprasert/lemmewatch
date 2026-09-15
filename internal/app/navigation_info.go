package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"lemmewatch/internal/metadata"
	"lemmewatch/internal/model"
	"lemmewatch/internal/textutil"
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
		lines := []string{n.media.Name, joinInfo(kind, year, infoRating(n.media.Rating), metadataText(n.media.Metadata, "runtime")), n.media.Summary}
		lines = appendDetail(lines, "Genres", metadataText(n.media.Metadata, "genres", "genre"))
		lines = appendDetail(lines, "Status", metadataText(n.media.Metadata, "status"))
		lines = appendDetail(lines, "Country", metadataText(n.media.Metadata, "country"))
		lines = appendDetail(lines, "Cast", metadataNames(n.media.Metadata, 6, "cast"))
		lines = appendDetail(lines, "Director", metadataNames(n.media.Metadata, 3, "director"))
		lines = appendDetail(lines, "Writer", metadataNames(n.media.Metadata, 3, "writer"))
		playedAt := n.playedAt
		if playedAt.IsZero() {
			playedAt = n.media.PlayedAt
		}
		if !playedAt.IsZero() {
			lines = append(lines, "Last played: "+playedAt.Local().Format("2006-01-02 15:04"))
		}
		return lines
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
		lines = append(lines, n.media.Name)
		return lines
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
		lines := []string{
			n.episode.Title,
			joinInfo(fmt.Sprintf("S%02dE%02d", n.episode.Season, n.episode.Episode), status),
			joinInfo(released, infoRating(n.episode.Rating)),
			n.media.Name,
		}
		if ratingLabel(n.episode.Rating) == "--" && ratingLabel(n.media.Rating) != "--" {
			lines[len(lines)-1] = joinInfo(n.media.Name, "Show rating "+n.media.Rating)
		}
		if summary := metadataText(n.episode.Metadata, "description", "overview"); summary != "" {
			lines = append(lines, summary)
		}
		lines = appendDetail(lines, "Guest cast", metadataNames(n.episode.Metadata, 6, "guestStars", "guest_stars"))
		return lines
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
			joinInfo(availability, n.stream.Provider, streamSource(n.stream.Source, quality, n.stream.Provider)),
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
		if len(n.stream.AudioLanguages) == 0 && len(n.stream.LanguageHints) > 0 {
			lines = append(lines, "Language hints: "+strings.Join(n.stream.LanguageHints, ", "))
		}
		lines = append(lines, torrentDetails(n.stream)...)
		return lines
	default:
		return []string{n.Label()}
	}
}

func appendDetail(lines []string, label, value string) []string {
	if value != "" {
		return append(lines, label+": "+value)
	}
	return lines
}

func streamSource(source, quality, provider string) string {
	var parts []string
	for _, part := range strings.Fields(textutil.Clean(source)) {
		if !strings.EqualFold(part, quality) {
			parts = append(parts, part)
		}
	}
	value := strings.Join(parts, " ")
	if strings.EqualFold(value, provider) {
		return ""
	}
	return value
}

// Pick one useful alias rather than dumping duplicate provider fields.
func metadataText(fields metadata.Fields, keys ...string) string {
	return metadataNames(fields, 0, keys...)
}

func metadataNames(fields metadata.Fields, limit int, keys ...string) string {
	for _, key := range keys {
		var values []string
		var value string
		if json.Unmarshal(fields[key], &value) == nil {
			values = []string{value}
		} else {
			_ = json.Unmarshal(fields[key], &values)
		}
		var clean []string
		for _, value := range values {
			value = textutil.Clean(value)
			if value == "" || value == "[url omitted]" || strings.EqualFold(value, "N/A") {
				continue
			}
			if !slices.Contains(clean, value) {
				clean = append(clean, value)
			}
		}
		if len(clean) == 0 {
			continue
		}
		if limit > 0 && len(clean) > limit {
			return strings.Join(clean[:limit], ", ") + fmt.Sprintf(" (+%d)", len(clean)-limit)
		}
		return strings.Join(clean, ", ")
	}
	return ""
}

func torrentDetails(stream model.Stream) []string {
	var lines []string
	state := metadataText(stream.TorrentMetadata, "download_state")
	if state != "" && !strings.EqualFold(state, "cached") && !strings.EqualFold(state, "completed") {
		lines = appendDetail(lines, "Download", state)
	}
	var progress float64
	if json.Unmarshal(stream.TorrentMetadata["progress"], &progress) == nil && progress > 0 && progress < 1 {
		lines = append(lines, fmt.Sprintf("Progress: %.0f%%", progress*100))
	}
	for _, fields := range []metadata.Fields{stream.TorrentMetadata, stream.CacheMetadata} {
		var files []metadata.Fields
		if json.Unmarshal(fields["files"], &files) != nil || len(files) == 0 {
			continue
		}
		if len(files) > 1 {
			lines = append(lines, "Pack: "+strconv.Itoa(len(files))+" files")
		}
		for _, file := range files {
			name := metadataText(file, "short_name", "name")
			name = strings.ReplaceAll(name, `\`, "/")
			if i := strings.LastIndex(name, "/"); i >= 0 {
				name = name[i+1:]
			}
			if len(files) != 1 && (stream.Filename == "" || !strings.EqualFold(name, stream.Filename)) {
				continue
			}
			mime := metadataText(file, "mimetype")
			switch mime {
			case "video/x-matroska":
				mime = "Matroska"
			case "video/mp4":
				mime = "MP4"
			case "video/webm":
				mime = "WebM"
			}
			lines = appendDetail(lines, "Format", mime)
			break
		}
		break
	}
	return lines
}

func infoRating(rating string) string {
	if ratingLabel(rating) == "--" {
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
