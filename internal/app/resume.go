package app

import (
	"fmt"
	"math"

	"lemmewatch/internal/config"
	"lemmewatch/internal/player"
)

func (n navigationChoice) resumeKey() string {
	if n.media.ID == "" {
		return ""
	}
	key := string(n.media.Type) + ":" + n.media.ID
	if n.episode.ID != "" {
		key += fmt.Sprintf(":%d:%d", n.episode.Season, n.episode.Episode)
	}
	return key
}

func (a App) resumePlayer(selected navigationChoice) player.Player {
	p := a.Player
	key := selected.resumeKey()
	if !p.SupportsResume() || a.Storage == nil || key == "" {
		return p
	}
	seconds, err := a.Storage.ResumePosition(key)
	if err != nil {
		_ = config.LogFailure("load resume position", err)
	}
	p.ResumeSeconds = max(0, seconds)
	last := seconds
	logged := false
	p.OnProgress = func(progress player.Progress) {
		if progress.Position < 1 || math.IsNaN(progress.Position) || math.IsInf(progress.Position, 0) {
			return // Startup and stopped states must not erase an existing checkpoint.
		}
		position := int(progress.Position)
		// Treat the last 15 seconds (at most 5% of short clips) as completed.
		if progress.Duration > 0 && progress.Position >= progress.Duration-math.Min(15, progress.Duration*0.05) {
			position = 0
		}
		if position == last {
			return
		}
		if err := a.Storage.SaveResumePosition(key, position); err != nil {
			if !logged {
				_ = config.LogFailure("save resume position", err)
				logged = true
			}
			return
		}
		last = position
	}
	return p
}
