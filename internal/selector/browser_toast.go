package selector

import (
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

type ToastKind string

const (
	ToastPlayback ToastKind = "playback"
	ToastLoad     ToastKind = "load"
	ToastSearch   ToastKind = "search"
	ToastHistory  ToastKind = "history"
	ToastSettings ToastKind = "settings"
	ToastSort     ToastKind = "sort"
	ToastEpisode  ToastKind = "episode"
	ToastStream   ToastKind = "stream"
)

const (
	toastInfoExpiry = 4 * time.Second
	toastErrExpiry  = 10 * time.Second
)

type toastID uint64

type toast struct {
	id   toastID
	kind ToastKind
	err  bool
	text string
}

type toastExpired struct{ id toastID }

type toastModel struct {
	current  *toast
	next     toastID
	spinning bool
	frame    int
	gen      uint64
}

func (t *toastModel) Set(kind ToastKind, format string, args ...any) toastID {
	return t.show(kind, false, fmt.Sprintf(format, args...))
}

func (t *toastModel) Err(kind ToastKind, format string, args ...any) toastID {
	return t.show(kind, true, fmt.Sprintf(format, args...))
}

func (t *toastModel) show(kind ToastKind, isErr bool, text string) toastID {
	t.next++
	t.gen++
	id := t.next
	t.current = &toast{id: id, kind: kind, err: isErr, text: text}
	return id
}

func (t *toastModel) Update(id toastID, text string) {
	if t.current == nil || t.current.id != id {
		return
	}
	t.current.text = text
}

func (t *toastModel) Clear() {
	if t.current == nil {
		return
	}
	t.current = nil
	t.gen++
}

func (t *toastModel) Spin(on bool) {
	if t.spinning == on {
		return
	}
	t.spinning = on
	t.frame = 0
}

func (t *toastModel) Expired(id toastID) bool {
	if t.current == nil || t.current.id != id {
		return false
	}
	t.current = nil
	t.gen++
	return true
}

func (t toastModel) Text() string {
	if t.current == nil {
		return ""
	}
	return t.current.text
}

func (m browserModel[T]) toastText() string { return m.toasts.Text() }
func (t toastModel) expiryCmd() tea.Cmd {
	if t.current == nil || t.spinning {
		return nil
	}
	id := t.current.id
	expiry := toastInfoExpiry
	if t.current.err {
		expiry = toastErrExpiry
	}
	return tea.Tick(expiry, func(time.Time) tea.Msg { return toastExpired{id: id} })
}

func (t toastModel) render(width int) string {
	if t.current == nil {
		return ""
	}
	text := t.current.text
	if t.spinning {
		text = spinnerFrames[t.frame%len(spinnerFrames)] + " " + text
	}
	return toastOverlay(text, width)
}

func spinnerCommand() tea.Cmd {
	return tea.Tick(80*time.Millisecond, func(time.Time) tea.Msg { return spinnerTick{} })
}

func toastOverlay(message string, width int) string {
	contentWidth := max(10, min(48, width-6))
	styled := toastBorder.Render(ansi.Truncate(plainLabel(message), contentWidth, "..."))
	return styled
}

type spinnerTick struct{}

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
