package selector

import tea "github.com/charmbracelet/bubbletea"

type metadataRootItem interface{ MetadataRoot() bool }

type metadataRefreshed[T item] struct {
	levels   []pane[T]
	right    pane[T]
	crumbs   []string
	loadID   uint64
	provider string
	err      error
}

func (m browserModel[T]) metadataRootLevel() int {
	if m.options.Refresh == nil {
		return -1
	}
	kind := m.activeInfoKey()
	if kind != "season" && kind != "episode" {
		return -1
	}
	for level := len(m.levels) - 1; level >= 0; level-- {
		items := m.filteredLevel(level)
		if len(items) == 0 {
			continue
		}
		selected := items[clamp(m.levels[level].index, len(items))].item
		if root, ok := any(selected).(metadataRootItem); ok && root.MetadataRoot() {
			return level
		}
	}
	return -1
}

// Refresh the series ancestor, then rebuild the path from its fresh children.
// Keep the last good panes intact until the entire refresh succeeds.
func (m browserModel[T]) refreshMetadata(root int) (tea.Model, tea.Cmd) {
	m.loading = true
	m.loadID++
	m.toasts.Clear()
	panes := append([]pane[T](nil), m.levels...)
	if m.focusRight {
		panes = append(panes, m.right)
	}
	selectedItems := m.filteredLevel(root)
	selected := selectedItems[clamp(panes[root].index, len(selectedItems))].item
	crumbs := append([]string(nil), m.crumbs...)
	return m, func() tea.Msg {
		msg := metadataRefreshed[T]{loadID: m.loadID, provider: m.provider}
		children, err := m.options.Refresh(m.ctx, selected)
		if err != nil {
			msg.err = err
			return msg
		}
		msg.levels = append([]pane[T](nil), panes[:root+1]...)
		msg.crumbs = append([]string(nil), crumbs[:min(root, len(crumbs))]...)
		msg.crumbs = append(msg.crumbs, plainLabel(selected.Label()))
		for level := root + 1; level < len(panes); level++ {
			fresh := pane[T]{title: m.childTitle(selected), items: children}
			old := filterItems(panes[level].items, panes[level].filter)
			found := false
			if len(old) > 0 {
				wanted := old[clamp(panes[level].index, len(old))].item
				for i, child := range children {
					if refreshIdentity(child) == refreshIdentity(wanted) {
						fresh.index, found = i, true
						break
					}
				}
			}
			msg.right = fresh
			if level == len(panes)-1 || !found {
				break
			}
			selected = children[fresh.index]
			children, err = m.load(m.ctx, selected)
			if err != nil {
				msg.err = err
				return msg
			}
			msg.levels = append(msg.levels, fresh)
			msg.crumbs = append(msg.crumbs, plainLabel(selected.Label()))
		}
		return msg
	}
}

func refreshIdentity[T item](value T) string {
	if cached, ok := any(value).(cacheableItem); ok && cached.CacheKey() != "" {
		return cached.CacheKey()
	}
	return value.Label()
}
