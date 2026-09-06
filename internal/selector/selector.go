package selector

import (
	"errors"
	"github.com/charmbracelet/lipgloss"
)

var ErrCancelled = errors.New("selection cancelled")

type item interface{ Label() string }

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"})
	selectedStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).Background(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#5A56E0"})
	inactiveSelected  = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6A6A6A", Dark: "#9E9E9E"}).Background(lipgloss.AdaptiveColor{Light: "#D8D8D8", Dark: "#3A3A3A"})
	hintStyle     = lipgloss.NewStyle().Faint(true)
)
