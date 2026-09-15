// Package languages normalizes language preferences and release metadata hints.
package languages

import (
	"fmt"
	"slices"
	"strings"

	"golang.org/x/text/language"
)

type Language struct {
	Code  string
	Name  string
	Flags string
}

// Flags follow Torrentio's mapping. India's flag is intentionally omitted:
// Torrentio uses it for Hindi, Telugu, and Tamil, so it cannot identify a language.
var Choices = []Language{
	{"en", "English", "🇬🇧 🇺🇸"}, {"ja", "Japanese", "🇯🇵"},
	{"es", "Spanish", "🇪🇸 🇲🇽"}, {"fr", "French", "🇫🇷"},
	{"de", "German", "🇩🇪"}, {"it", "Italian", "🇮🇹"},
	{"pt", "Portuguese", "🇵🇹 🇧🇷"}, {"zh", "Chinese", "🇨🇳 🇹🇼"},
	{"ko", "Korean", "🇰🇷"}, {"ru", "Russian", "🇷🇺"},
	{"hi", "Hindi", ""}, {"te", "Telugu", ""}, {"ta", "Tamil", ""},
	{"ar", "Arabic", "🇸🇦"}, {"nl", "Dutch", "🇳🇱"},
	{"pl", "Polish", "🇵🇱"}, {"tr", "Turkish", "🇹🇷"},
	{"uk", "Ukrainian", "🇺🇦"}, {"sv", "Swedish", "🇸🇪"},
	{"no", "Norwegian", "🇳🇴"}, {"da", "Danish", "🇩🇰"},
	{"fi", "Finnish", "🇫🇮"}, {"cs", "Czech", "🇨🇿"},
	{"sk", "Slovak", "🇸🇰"}, {"hu", "Hungarian", "🇭🇺"},
	{"ro", "Romanian", "🇷🇴"}, {"bg", "Bulgarian", "🇧🇬"},
	{"el", "Greek", "🇬🇷"}, {"he", "Hebrew", "🇮🇱"},
	{"fa", "Persian", "🇮🇷"}, {"vi", "Vietnamese", "🇻🇳"},
	{"id", "Indonesian", "🇮🇩"}, {"ms", "Malay", "🇲🇾"},
	{"th", "Thai", "🇹🇭"}, {"lt", "Lithuanian", "🇱🇹"},
	{"lv", "Latvian", "🇱🇻"}, {"et", "Estonian", "🇪🇪"},
	{"sl", "Slovenian", "🇸🇮"}, {"sr", "Serbian", "🇷🇸"},
	{"hr", "Croatian", "🇭🇷"},
}

func Normalize(value string) (string, error) {
	value = strings.TrimSpace(value)
	for _, choice := range Choices {
		if strings.EqualFold(value, choice.Name) {
			return choice.Code, nil
		}
	}
	tag, err := language.Parse(value)
	base, _, _ := tag.Raw()
	if err != nil || base.String() == "und" || strings.ContainsAny(value, "_, ") {
		return "", fmt.Errorf("use language names or ISO codes, e.g. ja,en")
	}
	return tag.String(), nil
}

func ParseList(value string) ([]string, error) {
	if strings.TrimSpace(value) == "" {
		return nil, nil
	}
	var result []string
	for part := range strings.SplitSeq(value, ",") {
		code, err := Normalize(part)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(result, code) {
			result = append(result, code)
		}
	}
	return result, nil
}

// Rank prefers the first matching language, then unknown, then known nonmatches.
// Region variants of the same language remain better than an unrelated language.
func Rank(available, preferred []string) int {
	if len(preferred) == 0 {
		return 0
	}
	best := len(preferred)*2 + 1
	if len(available) == 0 {
		return best - 1
	}
	for i, wanted := range preferred {
		wanted, err := Normalize(wanted)
		if err != nil {
			continue
		}
		for _, actual := range available {
			actual, err = Normalize(actual)
			if err != nil {
				continue
			}
			if actual == wanted {
				best = min(best, i*2)
			} else if strings.Split(actual, "-")[0] == strings.Split(wanted, "-")[0] {
				best = min(best, i*2+1)
			}
		}
	}
	return best
}
