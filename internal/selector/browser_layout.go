package selector

import (
	"fmt"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func validPaneSizes(count int, sizes []int) bool {
	if (count != 2 && count != 3) || len(sizes) != count {
		return false
	}
	for _, size := range sizes {
		if size < 1 || size > 1000 {
			return false
		}
	}
	return true
}

func paneSizeWeights(count int, sizes map[int][]int) []int {
	if configured := sizes[count]; validPaneSizes(count, configured) {
		return configured
	}
	if count == 2 {
		return []int{50, 50}
	}
	return []int{20, 40, 40}
}

func formatPaneSizes(sizes []int) string {
	parts := make([]string, len(sizes))
	for i, size := range sizes {
		parts[i] = strconv.Itoa(size)
	}
	return strings.Join(parts, ":")
}

func parsePaneSizes(value string, count int) ([]int, error) {
	parts := strings.Split(strings.NewReplacer("/", ":", ",", ":").Replace(value), ":")
	sizes := make([]int, len(parts))
	for i, part := range parts {
		sizes[i], _ = strconv.Atoi(strings.TrimSpace(part))
	}
	if !validPaneSizes(count, sizes) {
		return nil, fmt.Errorf("Enter %d sizes from 1 to 1000, separated by colons", count)
	}
	return sizes, nil
}

func (m browserModel[T]) updatePaneSizes(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.closeOverlay()
	case "enter":
		sizes, err := parsePaneSizes(m.paneSizeValue, m.paneSizeCount)
		if err != nil {
			m.toasts.Err(ToastSettings, "%s", err)
			return m, nil
		}
		if m.options.SavePaneSizes != nil {
			if err := m.options.SavePaneSizes(m.paneSizeCount, sizes); err != nil {
				m.toasts.Err(ToastSettings, "Could not save pane sizes")
				return m, nil
			}
		}
		if m.paneSizes == nil {
			m.paneSizes = make(map[int][]int)
		}
		m.paneSizes[m.paneSizeCount] = sizes
		m.closeOverlay()
		m.toasts.Clear()
	case "backspace", "ctrl+h":
		runes := []rune(m.paneSizeValue)
		if len(runes) > 0 {
			m.paneSizeValue = string(runes[:len(runes)-1])
		}
	case "ctrl+u":
		m.paneSizeValue = ""
	case "ctrl+c":
		return m, tea.Quit
	default:
		if msg.Type == tea.KeySpace {
			m.paneSizeValue += " "
		} else if msg.Type == tea.KeyRunes {
			m.paneSizeValue += string(msg.Runes)
		}
	}
	return m, nil
}
