package selector

import (
	"context"
	"errors"

	tea "github.com/charmbracelet/bubbletea"
)

type playFinished struct{ err error }

type playProgress struct {
	id   toastID
	text string
}

type playbackModel struct {
	active bool
	stop   context.CancelFunc
	ch     <-chan string
	toast  toastID
}

func (p *playbackModel) busy() bool { return p.active }

func (p *playbackModel) start(ch <-chan string, stop context.CancelFunc, toast toastID) {
	p.active = true
	p.stop = stop
	p.ch = ch
	p.toast = toast
}

func (p *playbackModel) finish() {
	p.active = false
	if p.stop != nil {
		p.stop = nil
	}
	p.ch = nil
	p.toast = 0
}

func (p *playbackModel) stopPlayback() bool {
	if p.stop == nil {
		return false
	}
	p.stop()
	return true
}

func (m *browserModel[T]) startPlayback(selected T) (tea.Model, tea.Cmd) {
	playContext, cancel := context.WithCancel(m.ctx)
	queued := false
	if stream, ok := any(selected).(streamItem); ok {
		info, isStream := stream.StreamInfo()
		queued = isStream && info.CacheApplicable && !info.Playable
	}
	text := "Starting playback..."
	if queued {
		text = "Queueing uncached torrent; playback starts after download..."
	}
	var ch <-chan string
	if m.options.Progress != nil {
		ch = m.options.Progress()
	}
	m.toasts.Spin(ch != nil)
	id := m.toasts.Set(ToastPlayback, "%s", text)
	m.playback.start(ch, cancel, id)
	playCmd := func() tea.Msg {
		return playFinished{err: m.options.Play(playContext, selected)}
	}
	if ch == nil {
		return *m, playCmd
	}
	return *m, tea.Batch(playCmd, listenProgress(ch, id))
}

func (m *browserModel[T]) updatePlaybackProgress(msg playProgress) (tea.Model, tea.Cmd) {
	if !m.playback.active || msg.id != m.playback.toast {
		return *m, nil
	}
	m.toasts.Update(msg.id, msg.text)
	return *m, listenProgress(m.playback.ch, msg.id)
}

func (m *browserModel[T]) updatePlaybackFinished(msg playFinished) (tea.Model, tea.Cmd) {
	m.playback.finish()
	m.toasts.Spin(false)
	switch {
	case msg.err == nil:
		m.toasts.Set(ToastPlayback, "Playback launched")
	case errors.Is(msg.err, context.Canceled):
		m.toasts.Set(ToastPlayback, "Playback stopped")
	default:
		m.toasts.Err(ToastPlayback, "Playback failed: %s", msg.err.Error())
	}
	return *m, nil
}

func listenProgress(ch <-chan string, id toastID) tea.Cmd {
	return func() tea.Msg {
		text, ok := <-ch
		if !ok {
			return nil
		}
		return playProgress{id: id, text: text}
	}
}
