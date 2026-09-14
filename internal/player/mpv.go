package player

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"time"

	"lemmewatch/internal/config"
)

func newMPVTracker() ([]string, *progressTracker, error) {
	endpoint, cleanup, err := mpvEndpoint()
	if err != nil {
		return nil, nil, err
	}
	return []string{"--input-ipc-server=" + endpoint}, &progressTracker{
		track: func(ctx context.Context, save func(Progress)) {
			trackMPV(ctx, endpoint, save)
		},
		close: cleanup,
	}, nil
}

func trackMPV(ctx context.Context, endpoint string, save func(Progress)) {
	started := time.Now()
	logged := false
	for ctx.Err() == nil {
		dialContext, cancel := context.WithTimeout(ctx, time.Second)
		conn, err := dialMPV(dialContext, endpoint)
		cancel()
		if err == nil {
			err = observeMPV(ctx, conn, save, time.Second)
		}
		if ctx.Err() != nil {
			return
		}
		if err != nil && !logged && time.Since(started) >= 15*time.Second {
			_ = config.LogFailure("playback progress", errors.New("mpv position unavailable; retrying the IPC connection"))
			logged = true
		}
		timer := time.NewTimer(time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}

type mpvEvent struct {
	Event     string          `json:"event"`
	Name      string          `json:"name"`
	Data      json.RawMessage `json:"data"`
	Reason    string          `json:"reason"`
	RequestID int             `json:"request_id"`
	Error     string          `json:"error"`
}

// observeMPV owns the connection and flushes the latest received position before returning.
func observeMPV(parent context.Context, conn net.Conn, save func(Progress), interval time.Duration) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	stop := context.AfterFunc(ctx, func() { _ = conn.Close() })
	defer stop()
	defer conn.Close()
	if err := conn.SetWriteDeadline(time.Now().Add(time.Second)); err != nil {
		return err
	}
	encoder := json.NewEncoder(conn)
	for index, name := range []string{"duration", "time-pos"} {
		if err := encoder.Encode(struct {
			Command   []any `json:"command"`
			RequestID int   `json:"request_id"`
		}{[]any{"observe_property", index + 1, name}, index + 1}); err != nil {
			return err
		}
	}
	// No read deadline: paused playback can legitimately produce no events for hours.
	events := make(chan mpvEvent)
	done := make(chan struct{})
	var readErr error
	go func() {
		defer close(done)
		scanner := bufio.NewScanner(conn)
		// Bound each message, not the lifetime traffic on this persistent connection.
		scanner.Buffer(make([]byte, 4096), 1<<20)
		for scanner.Scan() {
			var event mpvEvent
			if err := json.Unmarshal(scanner.Bytes(), &event); err != nil {
				readErr = err
				return
			}
			select {
			case events <- event:
			case <-ctx.Done():
				return
			}
		}
		readErr = scanner.Err()
		if readErr == nil {
			readErr = io.EOF
		}
	}()
	defer func() {
		cancel()
		_ = conn.Close()
		<-done
	}()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var latest Progress
	hasPosition, dirty := false, false
	flush := func() {
		if hasPosition && dirty {
			save(latest)
			dirty = false
		}
	}
	defer flush()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-done:
			return readErr
		case <-ticker.C:
			flush()
		case event := <-events:
			if event.RequestID != 0 && event.Error != "" && event.Error != "success" {
				return errors.New("mpv property observation failed")
			}
			switch event.Event {
			case "property-change":
				var value *float64
				if json.Unmarshal(event.Data, &value) != nil || value == nil || *value < 0 {
					continue // Unloaded properties must not overwrite the final position.
				}
				switch event.Name {
				case "duration":
					latest.Duration = *value
					dirty = true
				case "time-pos":
					latest.Position = *value
					dirty = true
					if !hasPosition {
						hasPosition = true
						flush()
					}
				}
			case "end-file":
				if event.Reason == "eof" && hasPosition && latest.Duration > 0 {
					latest.Position = latest.Duration
					dirty = true
				}
				flush()
			case "shutdown":
				return io.EOF
			}
		}
	}
}
