package player

import (
	"context"
	"fmt"
	"path"
	"strconv"
	"strings"
	"time"

	"lemmewatch/internal/config"
)

// Progress contains media time in seconds, not elapsed process time.
type Progress struct {
	Position float64
	Duration float64
}

type progressTracker struct {
	poll  func(context.Context) (Progress, error)
	track func(context.Context, func(Progress))
	close func()
}

func (p Player) SupportsResume() bool {
	switch p.name() {
	case "vlc", "mpv":
		return true
	default:
		return false
	}
}

func (p Player) name() string {
	return strings.TrimSuffix(strings.ToLower(path.Base(strings.ReplaceAll(p.Executable, `\`, "/"))), ".exe")
}

func (p Player) prepareTracking() ([]string, *progressTracker) {
	arguments := append([]string(nil), p.Arguments...)
	if !p.SupportsResume() || p.OnProgress == nil {
		return arguments, nil
	}
	var flags []string
	var tracker *progressTracker
	var err error
	switch p.name() {
	case "vlc":
		if p.ResumeSeconds > 0 {
			// VLC checkpoints are polled coarsely; rewind slightly to restore context.
			arguments = append(arguments, "--start-time="+strconv.Itoa(max(0, p.ResumeSeconds-5)))
		}
		flags, tracker, err = newVLCTracker()
	case "mpv":
		// Lemmewatch owns resume state, including restarting completed media.
		arguments = append(arguments, "--resume-playback=no")
		if p.ResumeSeconds > 0 {
			arguments = append(arguments, "--start="+strconv.Itoa(p.ResumeSeconds))
		}
		flags, tracker, err = newMPVTracker()
	}
	if err != nil {
		_ = config.LogFailure("playback progress", fmt.Errorf("could not initialize %s tracking", p.name()))
		return arguments, nil
	}
	return append(arguments, flags...), tracker
}

func (t *progressTracker) start(parent context.Context, save func(Progress)) func() {
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		if t.track != nil {
			t.track(ctx, save)
			return
		}
		timer := time.NewTimer(0)
		defer timer.Stop()
		started := time.Now()
		logged := false
		for {
			select {
			case <-ctx.Done():
				return
			case <-timer.C:
			}
			pollContext, stop := context.WithTimeout(ctx, time.Second)
			progress, err := t.poll(pollContext)
			stop()
			if ctx.Err() != nil {
				return
			}
			if err == nil {
				save(progress)
			} else if !logged && time.Since(started) >= 15*time.Second {
				_ = config.LogFailure("playback progress", fmt.Errorf("player position unavailable; playback can continue without new checkpoints"))
				logged = true
			}
			// Retry quickly while the player starts, then checkpoint roughly every five seconds.
			interval := 5 * time.Second
			if err != nil && time.Since(started) < 15*time.Second {
				interval = time.Second
			}
			timer.Reset(interval)
		}
	}()
	return func() {
		cancel()
		<-done
	}
}
