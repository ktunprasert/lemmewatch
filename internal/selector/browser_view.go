package selector

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func (m browserModel[T]) View() string {
	width, height := m.width, m.height
	if width <= 0 {
		width = 100
	}
	if height <= 0 {
		height = 24
	}
	rows := max(1, height-6)
	current := m.levels[len(m.levels)-1]
	panes := make([]visiblePane[T], 0, len(m.levels)+1)
	for i, level := range m.levels {
		title := level.title
		if i == 0 && len(m.options.ParentGroups) > 0 {
			title = groupTabs(m.options.ParentGroups, m.groupIndex)
		}
		panes = append(panes, visiblePane[T]{title: title, items: m.filteredLevel(i), index: level.index, filter: level.filter, active: !m.focusRight && i == len(m.levels)-1})
	}
	rightTitle := m.right.title
	if m.rightHasStreams() {
		qualityLabel := "All qualities"
		if m.quality > 0 {
			qualityLabel = fmt.Sprintf("%dp", m.quality)
		}
		if m.rightCacheApplicable() {
			cacheLabel := "Cached"
			if !m.cachedOnly {
				cacheLabel = "All"
			}
			rightTitle = fmt.Sprintf("Streams  [%s]  [%s]", cacheLabel, qualityLabel)
		} else {
			rightTitle = fmt.Sprintf("Streams  [%s]", qualityLabel)
		}
	}
	if rightTitle != "" || len(m.right.items) > 0 || m.loading && !m.searching || m.err != nil {
		panes = append(panes, visiblePane[T]{title: rightTitle, items: m.filteredRight(), index: m.right.index, filter: m.right.filter, active: m.focusRight, loading: m.loading && !m.searching, err: m.err})
	}
	visible, widths := paneLayout(width, panes)
	rendered := make([]string, len(visible))
	for i, pane := range visible {
		rendered[i] = renderBrowserPane(pane.title, pane.items, pane.index, widths[i], rows, pane.active, pane.filter, pane.loading, pane.err, m.mode, m.options.Watched)
	}
	breadcrumb := m.breadcrumb()
	helpText := renderHelpLine(m.help, width, m.shortHelp(browserKeys()), helpLineOptions{Right: m.options.Version, RightColumn: true})
	base := ansi.Truncate(breadcrumb, width, "…") + "\n" + lipgloss.JoinHorizontal(lipgloss.Top, rendered...) + "\n" + helpText + "\n"
	var modal string
	switch m.overlay {
	case overlayHelp:
		modal = m.helpModal()
	case overlayCustomPlayer:
		modal = inputModal("Custom player", m.customPlayerValue, 50, []key.Binding{
			hintBinding("enter", "save"),
			hintBinding("esc", "cancel"),
			hintBinding("ctrl-u", "clear"),
		})
	case overlayProviderAPIKey:
		modal = inputModal("TorBox API key", strings.Repeat("*", len([]rune(m.providerAPIKeyValue))), 50, []key.Binding{
			hintBinding("enter", "save"),
			hintBinding("esc", "cancel"),
			hintBinding("ctrl-w", "word"),
			hintBinding("ctrl-u", "clear"),
		})
	case overlaySettings:
		modal = m.settingsModal()
	case overlaySort:
		modal = sortModal(m.focusRight && m.rightHasStreams(), m.inHistoryRoot())
	case overlayMode:
		modal = modeModal(m.contextModes())
	case overlayQuery:
		modal = inputModal("Search", m.query, 64, []key.Binding{
			hintBinding("enter", "search"),
			hintBinding("esc", "cancel"),
			hintBinding("ctrl-w", "word"),
			hintBinding("ctrl-u", "clear"),
		})
	case overlayFilter:
		filter := current.filter
		if m.focusRight {
			filter = m.right.filter
		}
		modal = inputModal("Filter active pane", filter, 50, []key.Binding{
			hintBinding("enter", "apply"),
			hintBinding("esc", "clear"),
			hintBinding("ctrl-w", "word"),
			hintBinding("ctrl-u", "clear"),
		})
	}
	view := base
	if modal != "" {
		view = overlay(view, modal, width)
	}
	if m.searching {
		view = overlay(view, activityModal(m.toasts.frame, "Searching"), width)
	} else if m.historyBusy {
		view = overlay(view, activityModal(m.toasts.frame, "Updating watched state"), width)
	}
	if toast := m.toasts.render(width); toast != "" {
		view = overlayToast(view, toast, width)
	}
	return "\x1b]0;" + plainLabel(breadcrumb) + "\x07" + view
}

func paneLayout[T item](width int, panes []visiblePane[T]) ([]visiblePane[T], []int) {
	if len(panes) == 0 {
		return nil, nil
	}
	count := min(3, len(panes))
	if width < 64 {
		count = 1
	} else if width < 88 {
		count = min(2, count)
	}
	if count == 1 {
		active := len(panes) - 1
		for i := range panes {
			if panes[i].active {
				active = i
				break
			}
		}
		return panes[active : active+1], []int{max(18, width-2)}
	}
	visible := panes[len(panes)-count:]
	minimums := []int{24, 40}
	weights := []int{1, 2}
	if count == 3 {
		minimums = []int{24, 24, 40}
		weights = []int{1, 1, 2}
	}
	extra := max(0, width-sum(minimums))
	weightTotal := sum(weights)
	widths := make([]int, count)
	used := 0
	for i := range count {
		share := extra * weights[i] / weightTotal
		widths[i] = minimums[i] + share - 2
		used += share
	}
	widths[count-1] += extra - used
	return visible, widths
}

func sum(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}
	return total
}

func (m browserModel[T]) breadcrumb() string {
	parts := make([]string, 0, len(m.crumbs)+2)
	if len(m.options.ParentGroups) > 0 && m.groupIndex >= 0 && m.groupIndex < len(m.options.ParentGroups) {
		group := m.options.ParentGroups[m.groupIndex]
		if group != "" {
			parts = append(parts, strings.ToUpper(group[:1])+group[1:])
		}
	}
	if m.activeQuery != "" {
		parts = append(parts, m.activeQuery)
	} else if len(parts) == 0 {
		parts = append(parts, "Search")
	}
	parts = append(parts, m.crumbs...)
	return strings.Join(parts, " / ")
}

func sortModal(torrents, history bool) string {
	lines := []string{
		headerStyle.Render("Sort results"),
		"a   Name ascending",
		"A   Name descending",
		"y   Year ascending",
		"Y   Year descending",
		"d/r Default relevance",
	}
	if history {
		lines = append(lines[:len(lines)-1], "p   Date played ascending", "P   Date played descending", lines[len(lines)-1])
	}
	if torrents {
		lines = []string{
			headerStyle.Render("Sort streams"),
			"q   Quality ascending",
			"Q   Quality descending",
			"c   Cached first",
			"C   Uncached first",
			"n   Name ascending",
			"N   Name descending",
			"d/r Default ranking",
		}
	}
	lines = append(lines, "", renderHelpLine(newHelpModel(), 32, []key.Binding{
		hintBinding("esc", "cancel"),
	}, helpLineOptions{}))
	return activeBorder.Padding(0, 2).Render(strings.Join(lines, "\n"))
}

func overlay(base, modal string, width int) string {
	baseLines := strings.Split(strings.TrimSuffix(base, "\n"), "\n")
	modalLines := strings.Split(modal, "\n")
	modalWidth := lipgloss.Width(modal)
	x := max(0, (width-modalWidth)/2)
	y := max(0, (len(baseLines)-len(modalLines))/2)
	return overlayAt(baseLines, modalLines, width, x, y)
}

func overlayToast(base, styled string, width int) string {
	baseLines := strings.Split(strings.TrimSuffix(base, "\n"), "\n")
	toastLines := strings.Split(styled, "\n")
	x := max(0, width-lipgloss.Width(styled)-1)
	y := max(0, len(baseLines)-len(toastLines)-1)
	return overlayAt(baseLines, toastLines, width, x, y)
}

func overlayAt(baseLines, modalLines []string, width, x, y int) string {
	modalWidth := 0
	for _, line := range modalLines {
		modalWidth = max(modalWidth, lipgloss.Width(line))
	}
	for len(baseLines) < y+len(modalLines) {
		baseLines = append(baseLines, "")
	}
	for i, modalLine := range modalLines {
		baseLine := baseLines[y+i]
		left := ansi.Cut(baseLine, 0, x)
		if padding := x - lipgloss.Width(left); padding > 0 {
			left += strings.Repeat(" ", padding)
		}
		right := ansi.Cut(baseLine, x+modalWidth, width)
		baseLines[y+i] = left + modalLine + right
	}
	return strings.Join(baseLines, "\n") + "\n"
}

func groupTabs(groups []string, active int) string {
	labels := make([]string, len(groups))
	for i, group := range groups {
		label := strings.ToUpper(group[:1]) + group[1:]
		if label == "Movie" {
			label = "Movies"
		}
		if i == active {
			label = headerStyle.Render("● " + label)
		} else {
			label = hintStyle.Render("○ " + label)
		}
		labels[i] = label
	}
	return strings.Join(labels, "    ")
}

func renderBrowserPane[T item](title string, items []indexed[T], selected, width, rows int, active bool, filter string, loading bool, loadErr error, selectedModes map[string]string, watched map[string]bool) string {
	contentWidth := max(1, width-2)
	lines := []string{headerStyle.Render(ansi.Truncate(title, contentWidth, "…"))}
	if loading {
		lines = append(lines, "Loading...")
	} else if loadErr != nil {
		lines = append(lines, ansi.Truncate("Error: "+loadErr.Error(), contentWidth, "…"), "Press Enter to retry")
	} else if len(items) == 0 {
		message := "No items"
		if filter != "" {
			message = "No filter matches"
		}
		lines = append(lines, message)
	} else {
		selected = clamp(selected, len(items))
		start := max(0, min(selected-rows/2, len(items)-rows))
		end := min(len(items), start+rows)
		for i := start; i < end; i++ {
			label := plainLabel(items[i].item.Label())
			indicator := "  "
			if isWatched(items[i].item, watched) {
				indicator = "✓ "
			}
			if status, ok := any(items[i].item).(statusItem); ok {
				if value := status.Status(watched); value != "" {
					indicator = value + " "
				}
			}
			label = indicator + label
			context := ""
			if contextual, ok := any(items[i].item).(contextualItem); ok {
				modes := contextual.ContextModes()
				if len(modes) > 0 {
					group := modes[0].Group
					if group == "" {
						group = modes[0].Name
					}
					key := selectedModes[group]
					if key == "" {
						key = modes[0].Key
					}
					matched := false
					for _, mode := range modes {
						if mode.Key == key {
							context = plainLabel(mode.Value)
							matched = true
							break
						}
					}
					if !matched {
						context = plainLabel(modes[0].Value)
					}
				}
			}
			available := max(1, contentWidth-2)
			unavailable, isUnavailable := any(items[i].item).(unavailableItem)
			if context != "" {
				context = ansi.Truncate(context, max(1, available/2), "…")
				label = ansi.Truncate(label, max(1, available-lipgloss.Width(context)-1), "…")
				if isUnavailable && unavailable.Unavailable() {
					label = unavailableStyle.Render(label)
				}
				label += strings.Repeat(" ", max(1, available-lipgloss.Width(label)-lipgloss.Width(context))) + hintStyle.Render(context)
			} else {
				label = ansi.Truncate(label, available, "…")
				if isUnavailable && unavailable.Unavailable() {
					label = unavailableStyle.Render(label)
				}
			}
			row := "  " + label
			if i == selected {
				style := selectedStyle
				if !active {
					style = inactiveSelected
				}
				row = style.Width(contentWidth).Render("> " + label)
			}
			lines = append(lines, row)
		}
		lines[0] += hintStyle.Render(fmt.Sprintf("  %d-%d/%d", start+1, end, len(items)))
	}
	for len(lines) < rows+1 {
		lines = append(lines, "")
	}
	style := inactiveBorder
	if active {
		style = activeBorder
	}
	return style.Width(width).Height(rows + 1).Render(strings.Join(lines, "\n"))
}
