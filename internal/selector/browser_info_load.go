package selector

import (
	"context"
	tea "github.com/charmbracelet/bubbletea"
)

type infoKeyItem interface{ InfoKey() string }
type modeInfoItem interface{ ModeNeedsInfo(string) bool }

type infoLoaded[T item] struct {
	key       string
	item      T
	err       error
	requestID uint64
}

func infoIdentity(value any) string {
	if identified, ok := value.(infoKeyItem); ok {
		return identified.InfoKey()
	}
	return ""
}

func (m *browserModel[T]) cancelInfo() {
	if m.infoCancel != nil {
		m.infoCancel()
		m.infoCancel = nil
	}
	m.infoPending = ""
	m.infoRequestID++
}

func (m *browserModel[T]) invalidateInfo() {
	m.cancelInfo()
	m.infoItems = nil
	m.infoFailures = nil
}

// Metadata serves both info panels and row detail modes. Visible missing ratings
// load serially, with the focused item first, even when its info panel is closed.
// Request IDs prevent late responses from replacing newer details.
func (m *browserModel[T]) ensureInfo() tea.Cmd {
	if m.options.LoadInfo == nil {
		return nil
	}
	if m.loading || m.searching {
		if m.infoPending != "" {
			m.cancelInfo()
		}
		return nil
	}
	for _, selected := range m.infoCandidates() {
		key := infoIdentity(selected)
		if key == "" || m.infoFailures[key] {
			continue
		}
		if _, exists := m.infoItems[key]; exists {
			continue
		}
		if m.infoPending == key {
			return nil
		}
		if m.infoPending != "" {
			m.cancelInfo()
		}
		ctx, cancel := context.WithCancel(m.ctx)
		m.infoCancel, m.infoPending = cancel, key
		m.infoRequestID++
		id, load := m.infoRequestID, m.options.LoadInfo
		return func() tea.Msg {
			value, err := load(ctx, selected)
			return infoLoaded[T]{key: key, item: value, err: err, requestID: id}
		}
	}
	if m.infoPending != "" {
		m.cancelInfo()
	}
	return nil
}

func (m browserModel[T]) infoCandidates() []T {
	var candidates []T
	width := m.width
	if width <= 0 {
		width = 100
	}
	panes, _ := paneLayout(width, m.browserPanes(), m.paneSizes)
	needsMode := func(value T, kind string) bool {
		if value, ok := any(value).(modeInfoItem); ok {
			return value.ModeNeedsInfo(m.mode[kind])
		}
		return false
	}
	for _, p := range panes {
		if !p.active || len(p.items) == 0 {
			continue
		}
		selected := p.items[clamp(p.index, len(p.items))].item
		if p.info.open || needsMode(selected, p.kind) {
			candidates = append(candidates, selected)
		}
	}
	rows := browserRows(m.height)
	for _, p := range panes {
		listRows := rows - paneInfoRows(rows, p.info.open)
		start := max(0, min(clamp(p.index, len(p.items))-listRows/2, len(p.items)-listRows))
		end := min(len(p.items), start+listRows)
		for _, entry := range p.items[start:end] {
			if needsMode(entry.item, p.kind) {
				candidates = append(candidates, entry.item)
			}
		}
	}
	return candidates
}

func (m browserModel[T]) attachInfo(p *visiblePane[T]) {
	p.detailItems = m.infoItems
	if !p.info.open || len(p.items) == 0 {
		return
	}
	key := infoIdentity(p.items[clamp(p.index, len(p.items))].item)
	if key == "" {
		return
	}
	if value, exists := m.infoItems[key]; exists {
		p.infoItem = &value
	}
	p.infoLoading = m.infoPending == key
	p.infoFailed = m.infoFailures[key]
}
