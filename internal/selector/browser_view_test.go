package selector

import (
	"fmt"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

func TestBrowserBorderTitlesAndViewportBounds(t *testing.T) {
	for _, width := range []int{40, 63, 64, 80, 87, 88, 120, 200} {
		for _, height := range []int{8, 24, 42} {
			t.Run(fmt.Sprintf("%dx%d", width, height), func(t *testing.T) {
				m := newBrowser(testChoice{label: "Series"})
				m.width, m.height = width, height
				m.levels = append(m.levels,
					pane[testChoice]{title: "Seasons", items: []testChoice{{label: "Season 1"}}},
					pane[testChoice]{title: "Episodes", items: []testChoice{{label: strings.Repeat("界", 100)}}},
				)
				m.right = pane[testChoice]{title: "Streams", items: []testChoice{{label: strings.Repeat("界", 100), terminal: true, cached: true}}}
				m.focusRight = true
				lines := strings.Split(strings.TrimSuffix(ansi.Strip(m.View()), "\n"), "\n")
				if len(lines) != height {
					t.Fatalf("height = %d, want %d", len(lines), height)
				}
				for i, line := range lines {
					if got := lipgloss.Width(line); got > width {
						t.Fatalf("line %d width = %d, want <= %d: %q", i, got, width, line)
					}
				}
				if !strings.Contains(lines[1], "╭─ Streams") || !strings.Contains(lines[2], "界") {
					t.Fatalf("title not embedded in border or first row wasted: %q", lines[:3])
				}
			})
		}
	}
}

func TestPaneLayoutKeepsFocusVisibleAndWidthsProportional(t *testing.T) {
	panes := []visiblePane[testChoice]{{title: "Media"}, {title: "Seasons"}, {title: "Episodes"}, {title: "Streams", active: true}}
	_, widths := paneLayout(120, panes)
	if widths[0] >= widths[1] || widths[2] != widths[1] {
		t.Fatalf("left parent not collapsed or main panes not equal: %v", widths)
	}
	panes[3].active, panes[0].active = false, true
	visible, widths := paneLayout(120, panes)
	if !visible[0].active || paneTitles(visible) != "Media,Seasons,Episodes" || paneWidth(widths) != 120 {
		t.Fatalf("focus fell outside viewport: %v %v", visible, widths)
	}
}

func TestPaneLayoutUsesFixedPositionalSplits(t *testing.T) {
	for _, count := range []int{2, 3} {
		panes := []visiblePane[testChoice]{{title: "History"}, {title: "Seasons"}}
		if count == 3 {
			panes = append(panes, visiblePane[testChoice]{title: "Episodes"})
		}
		for _, width := range []int{88, 89, 120, 136, 200} {
			_, widths := paneLayout(width, panes)
			if paneWidth(widths) != width {
				t.Fatalf("%d panes at %d columns: viewport mismatch %v", count, width, widths)
			}
			main := 0
			if count == 3 {
				main = 1
				if widths[0]+2 != width/5 {
					t.Fatalf("left parent not at 20%%: %v", widths)
				}
			}
			if gap := widths[main+1] - widths[main]; gap < 0 || gap > 1 {
				t.Fatalf("main panes not evenly split: %v", widths)
			}
			for focused := range panes {
				for i := range panes {
					panes[i].active = i == focused
					panes[i].info.open = true
					panes[i].kind = "stream"
				}
				_, changed := paneLayout(width, panes)
				for i := range widths {
					if changed[i] != widths[i] {
						t.Fatalf("focus, info, or content shifted widths: %v -> %v", widths, changed)
					}
				}
			}
		}
	}
}

func TestEpisodeNumberSurvivesLongTitleAndContext(t *testing.T) {
	for _, number := range []int{6, 10, 11, 12, 100} {
		prefix := fmt.Sprintf("Episode %d", number)
		items := []indexed[testChoice]{{item: testChoice{prefix: prefix, label: strings.Repeat("Long title ", 10), modes: []ContextMode{{Key: "a", Value: "2024-09-24"}}}}}
		view := ansi.Strip(renderBrowserPane(visiblePane[testChoice]{title: "Episodes", items: items, active: true}, 26, 6, nil, nil))
		row := strings.Split(view, "\n")[1]
		if !strings.Contains(row, prefix+" ") || !strings.Contains(row, "…") || lipgloss.Width(row) != 28 {
			t.Fatalf("episode identifier lost or row overflowed: %q", row)
		}
	}
}
