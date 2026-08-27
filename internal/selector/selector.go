package selector

import (
	"errors"
	"github.com/charmbracelet/lipgloss"
)

var ErrCancelled = errors.New("selection cancelled")

type item interface{ Label() string }

var (
	headerStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#7D7AFF"})
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).Background(lipgloss.AdaptiveColor{Light: "#5A56E0", Dark: "#5A56E0"})
	hintStyle     = lipgloss.NewStyle().Faint(true)
)
