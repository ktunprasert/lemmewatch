package selector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestInfoLoadsOnDemandAndRejectsStaleSelection(t *testing.T) {
	m := newBrowser(testChoice{label: "First", cacheKey: "first"}, testChoice{label: "Second", cacheKey: "second"})
	loads := 0
	m.options.LoadInfo = func(ctx context.Context, item testChoice) (testChoice, error) {
		loads++
		item.infoLines = []string{"Full metadata for " + item.label}
		return item, ctx.Err()
	}
	next, cmd := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = next.(browserModel[testChoice])
	if cmd != nil || loads != 0 {
		t.Fatal("closed info fetched metadata")
	}
	next, first := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	if first == nil || !strings.Contains(ansi.Strip(m.View()), "Loading additional metadata") {
		t.Fatal("info request missing")
	}
	next, second := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = next.(browserModel[testChoice])
	if second == nil {
		t.Fatal("selection did not request new info")
	}
	stale := first().(infoLoaded[testChoice])
	if !errors.Is(stale.err, context.Canceled) {
		t.Fatal("old request was not cancelled")
	}
	next, _ = m.Update(stale)
	m = next.(browserModel[testChoice])
	if m.infoFailures["first"] {
		t.Fatal("stale request changed state")
	}
	next, cmd = m.Update(second())
	m = next.(browserModel[testChoice])
	if cmd != nil || !strings.Contains(ansi.Strip(m.View()), "Full metadata for Second") {
		t.Fatal("loaded info missing or refetched")
	}
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	next, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	if cmd != nil || loads != 2 {
		t.Fatal("reopening cached info fetched again")
	}
	if len(m.current().items[1].infoLines) != 0 {
		t.Fatal("info mutated navigation data")
	}
}

func TestRatingModeLoadsVisibleRowsWithInfoClosed(t *testing.T) {
	items := make([]testChoice, 40)
	for i := range items {
		items[i] = testChoice{label: fmt.Sprintf("Show %02d", i), cacheKey: fmt.Sprint(i), modes: []ContextMode{{Group: "media", Key: "r", Name: "Rating", Value: "--"}}}
	}
	m := newBrowser(items...)
	m.width, m.height = 80, 12
	m.current().index = 20
	m.mode = map[string]string{"media": "r"}
	var requested []string
	m.options.LoadInfo = func(_ context.Context, item testChoice) (testChoice, error) {
		requested = append(requested, item.cacheKey)
		item.label = "Canonical title"
		item.modes = []ContextMode{{Group: "media", Key: "r", Name: "Rating", Value: "7.0"}}
		return item, nil
	}
	cmd := m.ensureInfo()
	for attempts := 0; cmd != nil && attempts < 20; attempts++ {
		next, nextCmd := m.Update(cmd())
		m = next.(browserModel[testChoice])
		cmd = nextCmd
	}
	want := []string{"20", "16", "17", "18", "19", "21", "22", "23"}
	if !slices.Equal(requested, want) {
		t.Fatalf("requested = %v, want %v", requested, want)
	}
	view := ansi.Strip(m.View())
	if strings.Count(view, "7.0") != 8 || !strings.Contains(view, "Show 20") || strings.Contains(view, "Canonical title") || strings.Contains(view, "├─ Info") {
		t.Fatal(view)
	}
	if cmd != nil || m.ensureInfo() != nil {
		t.Fatal("rating mode refetched cached rows")
	}
}

func TestChangingDetailModeCancelsRatingLookup(t *testing.T) {
	m := newBrowser(testChoice{label: "Show", cacheKey: "show", modes: []ContextMode{{Group: "media", Key: "r", Name: "Rating", Value: "--"}}})
	m.mode = map[string]string{"media": "r"}
	m.options.LoadInfo = func(ctx context.Context, item testChoice) (testChoice, error) { return item, ctx.Err() }
	cmd := m.ensureInfo()
	if cmd == nil {
		t.Fatal("rating mode did not load")
	}
	m.mode["media"] = "y"
	if m.ensureInfo() != nil {
		t.Fatal("other mode still loads ratings")
	}
	if msg := cmd().(infoLoaded[testChoice]); !errors.Is(msg.err, context.Canceled) {
		t.Fatal("rating request not cancelled")
	}
}

func TestInfoFailureKeepsBasicDataAndCanRetry(t *testing.T) {
	m := newBrowser(testChoice{label: "Show", cacheKey: "show", infoLines: []string{"Cached summary"}})
	m.options.LoadInfo = func(_ context.Context, item testChoice) (testChoice, error) { return item, errors.New("network") }
	next, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	next, cmd = m.Update(cmd())
	m = next.(browserModel[testChoice])
	if cmd != nil {
		t.Fatal("failure caused retry loop")
	}
	view := ansi.Strip(m.View())
	if !strings.Contains(view, "Cached summary") || !strings.Contains(view, "Additional metadata unavailable") {
		t.Fatal(view)
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	m = next.(browserModel[testChoice])
	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'i'}})
	if cmd == nil {
		t.Fatal("reopening did not retry")
	}
}

func TestInvalidatedInfoDiscardsLateResponse(t *testing.T) {
	m := newBrowser(testChoice{label: "Show", cacheKey: "show"})
	m.options.LoadInfo = func(_ context.Context, item testChoice) (testChoice, error) { return item, nil }
	m.toggleInfo()
	cmd := m.ensureInfo()
	stale := cmd()
	m.invalidateInfo()
	next, _ := m.Update(stale)
	m = next.(browserModel[testChoice])
	if len(m.infoItems) != 0 {
		t.Fatal("stale result repopulated refreshed info")
	}
}

func TestBrowserSanitizesExternalTextWithoutBreakingFrames(t *testing.T) {
	m := newBrowser(testChoice{label: "Title 🔥  日本語", infoLines: []string{"Cast: 👋🏽  Alice\tBob\u202e\u200b"}})
	m.width, m.height, m.activeQuery = 64, 24, "Query 🇯🇵\t  text"
	m.toggleInfo()
	view := ansi.Strip(m.View())
	for _, bad := range []string{"🔥", "👋", "🏽", "🇯🇵", "\u202e", "\u200b", "\t"} {
		if strings.Contains(view, bad) {
			t.Fatalf("unsafe text %q retained", bad)
		}
	}
	if !strings.Contains(view, "Cast: Alice Bob") || !strings.Contains(view, "Title 日本語") || !strings.Contains(view, "Query text") {
		t.Fatal(view)
	}
	for _, line := range strings.Split(view, "\n") {
		if ansi.StringWidth(line) > m.width {
			t.Fatalf("overflow: %q", line)
		}
	}
}
