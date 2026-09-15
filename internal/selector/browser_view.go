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
	rows := browserRows(height)
	current := m.levels[len(m.levels)-1]
	visible, widths := paneLayout(width, m.browserPanes(), m.paneSizes)
	rendered := make([]string, len(visible))
	for i, pane := range visible {
		rendered[i] = renderBrowserPane(pane, widths[i], rows, m.mode, m.options.Watched)
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
	case overlayPlaybackSetting:
		title := "Audio languages (first → fallback: ja,en)"
		if m.settingsIndex == 12 {
			title = "Subtitle languages (first → fallback: en,ja)"
		}
		if m.settingsIndex == 13 {
			title = "Playback speed (0.25–4; empty = default)"
		}
		modal = inputModal(title, m.playbackSettingValue, 54, []key.Binding{
			hintBinding("enter", "save"), hintBinding("esc", "cancel"), hintBinding("ctrl-u", "default"),
		})
	case overlayPaneSizes:
		modal = inputModal(fmt.Sprintf("%d-pane sizes", m.paneSizeCount), m.paneSizeValue, 50, []key.Binding{
			hintBinding("enter", "save"),
			hintBinding("esc", "cancel"),
			hintBinding("ctrl-u", "clear"),
		})
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

func (m browserModel[T]) browserPanes() []visiblePane[T] {
	panes := make([]visiblePane[T], 0, len(m.levels)+1)
	for i, level := range m.levels {
		title := level.title
		if i == 0 && len(m.options.ParentGroups) > 0 {
			title = groupTabs(m.options.ParentGroups, m.groupIndex)
		}
		kind := paneKind(level)
		panes = append(panes, visiblePane[T]{title: title, kind: kind, info: m.info[kind], items: m.filteredLevel(i), index: level.index, filter: level.filter, active: !m.focusRight && i == len(m.levels)-1})
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
		kind := paneKind(m.right)
		panes = append(panes, visiblePane[T]{title: rightTitle, kind: kind, info: m.info[kind], items: m.filteredRight(), index: m.right.index, filter: m.right.filter, active: m.focusRight, loading: m.loading && !m.searching, err: m.err})
	}
	return panes
}

func paneLayout[T item](width int, panes []visiblePane[T], sizes map[int][]int) ([]visiblePane[T], []int) {
	if len(panes) == 0 {
		return nil, nil
	}
	count := min(3, len(panes))
	if width < 64 {
		count = 1
	} else if width < 88 {
		count = min(2, count)
	}
	active := len(panes) - 1
	for i := range panes {
		if panes[i].active {
			active = i
			break
		}
	}
	if count == 1 {
		return panes[active : active+1], []int{max(18, width-2)}
	}
	start := min(active, len(panes)-count)
	visible := panes[start : start+count]
	weights := paneSizeWeights(count, sizes)
	total := 0
	for _, weight := range weights {
		total += weight
	}
	widths := make([]int, count)
	used := 0
	for i, weight := range weights {
		widths[i] = max(18, width*weight/total)
		used += widths[i]
	}
	// Borrow space from the widest pane when a small ratio hits the minimum.
	for used > width {
		largest := 0
		for i := range widths {
			if widths[i] > widths[largest] {
				largest = i
			}
		}
		widths[largest]--
		used--
	}
	widths[count-1] += width - used
	for i := range widths {
		widths[i] -= 2 // Border cells.
	}
	return visible, widths
}

func browserRows(height int) int {
	if height <= 0 {
		height = 24
	}
	return max(1, height-4)
}

func paneKind[T item](p pane[T]) string {
	if len(p.items) > 0 {
		if contextual, ok := any(p.items[0]).(contextualItem); ok {
			if modes := contextual.ContextModes(); len(modes) > 0 && modes[0].Group != "" {
				return modes[0].Group
			}
		}
	}
	switch p.title {
	case "Seasons":
		return "season"
	case "Episodes":
		return "episode"
	case "Streams":
		return "stream"
	default:
		return "media"
	}
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
	for i := range parts {
		parts[i] = plainLabel(parts[i])
	}
	return strings.Join(parts, " / ")
}

func sortModal(torrents, history bool) string {
	title := "Sort results"
	lines := []string{
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
		title = "Sort streams"
		lines = []string{
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
	return titledModal(title, strings.Join(lines, "\n"), activeBorder.Padding(0, 2))
}

func titledModal(title, content string, style lipgloss.Style) string {
	lines := strings.Split(style.Render(content), "\n")
	lines[0] = paneRule(title, lipgloss.Width(lines[0])-2, "╭", "╮", true)
	return strings.Join(lines, "\n")
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
	y := max(0, len(baseLines)-len(toastLines)-3)
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

func renderBrowserPane[T item](pane visiblePane[T], width, rows int, selectedModes map[string]string, watched map[string]bool) string {
	title, items, selected := pane.title, pane.items, pane.index
	active, filter, loading, loadErr := pane.active, pane.filter, pane.loading, pane.err
	infoRows := paneInfoRows(rows, pane.info.open)
	listRows := rows - infoRows
	contentWidth := max(1, width)
	lines := make([]string, 0, rows)
	if loading {
		lines = append(lines, "Loading...")
	} else if loadErr != nil {
		lines = append(lines, ansi.Truncate("Error: "+plainLabel(loadErr.Error()), contentWidth, "…"), "Press Enter to retry")
	} else if len(items) == 0 {
		message := "No items"
		if filter != "" {
			message = "No filter matches"
		}
		lines = append(lines, message)
	} else {
		selected = clamp(selected, len(items))
		start := max(0, min(selected-listRows/2, len(items)-listRows))
		end := min(len(items), start+listRows)
		for i := start; i < end; i++ {
			label := plainLabel(items[i].item.Label())
			prefix := ""
			if split, ok := any(items[i].item).(rowLabelItem); ok {
				prefix, label = split.RowLabel()
				prefix, label = plainLabel(prefix), plainLabel(label)
			}
			indicator := "  "
			if isWatched(items[i].item, watched) {
				indicator = "✓ "
			}
			if status, ok := any(items[i].item).(statusItem); ok {
				if value := status.Status(watched); value != "" {
					indicator = ansi.Truncate(plainLabel(value), 1, "") + " "
				}
			}
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
			minimumLabel := lipgloss.Width(indicator + prefix)
			if label != "" {
				minimumLabel += 2 // A separator and at least an ellipsis.
			}
			contextWidth := min(available/2, available-minimumLabel-1)
			if contextWidth < 3 || prefix != "" && label == "" && lipgloss.Width(context) > contextWidth {
				context = ""
			} else {
				context = ansi.Truncate(context, contextWidth, "…")
			}
			labelWidth := available
			if context != "" {
				labelWidth -= lipgloss.Width(context) + 1
			}
			if prefix != "" && label != "" {
				prefix += " "
			}
			label = indicator + prefix + ansi.Truncate(label, max(0, labelWidth-lipgloss.Width(indicator+prefix)), "…")
			label = ansi.Truncate(label, labelWidth, "…")
			unavailable, isUnavailable := any(items[i].item).(unavailableItem)
			if isUnavailable && unavailable.Unavailable() {
				label = unavailableStyle.Render(label)
			}
			if context != "" {
				label += strings.Repeat(" ", max(1, available-lipgloss.Width(label)-lipgloss.Width(context))) + hintStyle.Render(context)
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
	}
	for len(lines) < listRows {
		lines = append(lines, "")
	}
	lines = lines[:listRows]
	infoTitle := "Info"
	if infoRows > 0 {
		details, content := paneInfoLines(pane, width, watched)
		offset := infoOffset(pane.info, content, len(details), infoRows-1)
		if len(details) > infoRows-1 {
			infoTitle += " · alt+j/k"
		}
		lines = append(lines, "") // Replaced with the info divider after framing.
		for i := range infoRows - 1 {
			line := ""
			if offset+i < len(details) {
				line = " " + details[offset+i]
			}
			lines = append(lines, line)
		}
	}
	style := inactiveBorder
	if active {
		style = activeBorder
	}
	if len(items) > 0 && !loading && loadErr == nil {
		title += fmt.Sprintf(" · %d/%d", clamp(selected, len(items))+1, len(items))
	}
	for i := range lines {
		lines[i] = ansi.Truncate(lines[i], contentWidth, "…")
	}
	frame := strings.Split(style.Width(width).Height(rows).Render(strings.Join(lines, "\n")), "\n")
	frame[0] = paneRule(title, width, "╭", "╮", active)
	if infoRows > 0 {
		frame[listRows+1] = paneRule(infoTitle, width, "├", "┤", active)
	}
	return strings.Join(frame, "\n")
}

func paneRule(title string, width int, left, right string, active bool) string {
	border := inactiveBorder
	labelStyle := hintStyle
	if active {
		border = activeBorder
		labelStyle = headerStyle
	}
	ruleStyle := lipgloss.NewStyle().Foreground(border.GetBorderTopForeground())
	label := ansi.Truncate(" "+plainLabel(title)+" ", max(0, width-1), "…")
	return ruleStyle.Render(left+"─") + labelStyle.Render(label) + ruleStyle.Render(strings.Repeat("─", max(0, width-1-lipgloss.Width(label)))+right)
}
