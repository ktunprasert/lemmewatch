package selector

import (
	"errors"

	"github.com/charmbracelet/lipgloss"
)

var ErrCancelled = errors.New("selection cancelled")

type item interface{ Label() string }

var (
	accentColor      = lipgloss.AdaptiveColor{Light: "#df6464", Dark: "#df6464"}
	headerStyle      = lipgloss.NewStyle().Bold(true).Foreground(accentColor)
	versionStyle     = lipgloss.NewStyle().Foreground(accentColor).Underline(true)
	selectedStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.AdaptiveColor{Light: "#FFFFFF", Dark: "#FFFFFF"}).Background(accentColor)
	inactiveSelected = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "#6A6A6A", Dark: "#9E9E9E"}).Background(lipgloss.AdaptiveColor{Light: "#D8D8D8", Dark: "#3A3A3A"})
	hintStyle        = lipgloss.NewStyle().Faint(true)
)
