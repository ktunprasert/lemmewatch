package textutil

import (
	"github.com/charmbracelet/x/ansi"
	"testing"
)

func TestCleanTerminalText(t *testing.T) {
	for _, tc := range []struct{ input, want string }{
		{"  Show\t\n  title\u00a0\u2003here ", "Show title here"},
		{"Show 👋🏽 🇯🇵 👨‍👩‍👧‍👦 ☀️ 1080p", "Show 1080p"},
		{"Wide ⬛ ⬜ symbols", "Wide symbols"},
		{"Cafe\u0301 日本語", "Café 日本語"},
		{"\u0301Title \u0301text", "Title text"},
		{"A\x1b[31mB\x1b[0m\x1b]0;injected\a\u202eC\u200bD", "ABCD"},
		{"⠋ Loading → metadata", "⠋ Loading → metadata"},
		{"1️⃣ episode", "1 episode"},
	} {
		got := Clean(tc.input)
		if got != tc.want {
			t.Errorf("Clean(%q) = %q; want %q", tc.input, got, tc.want)
		}
		if got != Clean(got) {
			t.Errorf("not idempotent: %q", got)
		}
	}
	if ansi.StringWidth(Clean("日本語")) != 6 {
		t.Fatal("ordinary wide text lost its measured cell width")
	}
}
