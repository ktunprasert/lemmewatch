// Package textutil prepares external text for fixed-column terminal displays.
package textutil

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
	"golang.org/x/text/unicode/norm"
)

// Clean removes emoji, terminal escapes, invisible formatting controls and
// variable-width whitespace. Ordinary international text remains intact; the
// renderer measures and truncates it by terminal cells rather than byte length.
func Clean(value string) string {
	runes := []rune(norm.NFC.String(ansi.Strip(value)))
	var out strings.Builder
	hasBase := false
	for i, r := range runes {
		if unicode.IsSpace(r) {
			out.WriteByte(' ')
			hasBase = false
			continue
		}
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r >= 0xfe00 && r <= 0xfe0f || r >= 0xe0100 && r <= 0xe01ef || r == 0x20e3 {
			continue
		}
		if emoji(r) || unicode.IsSymbol(r) && (ansi.StringWidth(string(r)) > 1 || i+1 < len(runes) && runes[i+1] == 0xfe0f) {
			out.WriteByte(' ')
			hasBase = false
			continue
		}
		if unicode.IsMark(r) && !hasBase {
			continue
		}
		out.WriteRune(r)
		if !unicode.IsMark(r) {
			hasBase = ansi.StringWidth(string(r)) > 0
		}
	}
	return strings.Join(strings.Fields(out.String()), " ")
}

func emoji(r rune) bool {
	return r >= 0x1f000 && r <= 0x1faff || r >= 0x2600 && r <= 0x27bf ||
		r >= 0x231a && r <= 0x231b || r >= 0x23e9 && r <= 0x23f3 ||
		r >= 0x23f8 && r <= 0x23fa || r >= 0x25fd && r <= 0x25fe ||
		r == 0x2b50 || r == 0x2b55 || r == 0x3030 || r == 0x303d || r == 0x3297 || r == 0x3299
}
