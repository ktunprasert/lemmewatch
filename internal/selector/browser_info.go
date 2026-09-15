package selector

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

type infoItem interface {
	InfoLines(watched map[string]bool) []string
}

type infoMergeItem[T item] interface{ WithInfo(T) T }

func mergeInfo[T item](current, full T) T {
	if merger, ok := any(current).(infoMergeItem[T]); ok {
		return merger.WithInfo(full)
	}
	return full
}

type paneInfoState struct {
	open    bool
	offset  int
	content string
}

func (m browserModel[T]) activeInfoKey() string {
	if m.focusRight {
		return paneKind(m.right)
	}
	return paneKind(m.levels[len(m.levels)-1])
}

func (m *browserModel[T]) toggleInfo() {
	if m.info == nil {
		m.info = make(map[string]paneInfoState)
	}
	key := m.activeInfoKey()
	m.info[key] = paneInfoState{open: !m.info[key].open}
	if m.info[key].open {
		m.infoFailures = nil
	}
}

func paneInfoRows(rows int, open bool) int {
	if !open || rows < 3 {
		return 0
	}
	return max(2, rows/3)
}

func paneInfoLines[T item](pane visiblePane[T], width int, watched map[string]bool) ([]string, string) {
	values := []string{"No item selected"}
	if pane.loading {
		values = []string{"Loading…"}
	} else if pane.err != nil {
		values = []string{"No item details available"}
	} else if len(pane.items) > 0 {
		selected := pane.items[clamp(pane.index, len(pane.items))].item
		if pane.infoItem != nil {
			selected = mergeInfo(selected, *pane.infoItem)
		}
		values = []string{selected.Label()}
		if detailed, ok := any(selected).(infoItem); ok {
			if info := detailed.InfoLines(watched); len(info) > 0 {
				values = info
			}
		}
	}
	if pane.infoLoading {
		values = append(values, "Loading additional metadata...")
	}
	if pane.infoFailed {
		values = append(values, "Additional metadata unavailable; close and reopen info to retry")
	}
	var lines, clean []string
	for _, value := range values {
		if value = plainLabel(value); value != "" {
			clean = append(clean, value)
			lines = append(lines, strings.Split(ansi.Wrap(value, max(1, width-2), ""), "\n")...)
		}
	}
	return lines, strings.Join(clean, "\n")
}

func infoOffset(state paneInfoState, content string, count, rows int) int {
	if state.content != content {
		return 0
	}
	return min(state.offset, max(0, count-rows))
}

func (m *browserModel[T]) scrollInfo(delta int) {
	width := m.width
	if width <= 0 {
		width = 100
	}
	panes, widths := paneLayout(width, m.browserPanes(), m.paneSizes)
	for i, pane := range panes {
		rows := paneInfoRows(browserRows(m.height), pane.info.open) - 1
		if !pane.active || rows <= 0 {
			continue
		}
		lines, content := paneInfoLines(pane, widths[i], m.options.Watched)
		state := pane.info
		state.offset = min(max(0, infoOffset(state, content, len(lines), rows)+delta), max(0, len(lines)-rows))
		state.content = content
		m.info[pane.kind] = state
		return
	}
}
